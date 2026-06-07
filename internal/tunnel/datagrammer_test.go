package tunnel

import (
	"context"
	"encoding/binary"
	"os"
	"sync"
	"testing"
	"time"

	"golang.zx2c4.com/wireguard/tun"
)

// fakeTUN is an in-memory tun.Device: packets pushed to inbound are returned by
// Read (as if the kernel sent them); packets written land on outbound.
type fakeTUN struct {
	inbound   chan []byte
	outbound  chan []byte
	closeOnce sync.Once
	closed    chan struct{}
	events    chan tun.Event
}

func newFakeTUN() *fakeTUN {
	return &fakeTUN{
		inbound:  make(chan []byte, 16),
		outbound: make(chan []byte, 16),
		closed:   make(chan struct{}),
		events:   make(chan tun.Event),
	}
}

func (f *fakeTUN) Read(bufs [][]byte, sizes []int, offset int) (int, error) {
	select {
	case <-f.closed:
		return 0, os.ErrClosed
	case p := <-f.inbound:
		sizes[0] = copy(bufs[0][offset:], p)
		return 1, nil
	}
}

func (f *fakeTUN) Write(bufs [][]byte, offset int) (int, error) {
	for _, b := range bufs {
		pkt := append([]byte(nil), b[offset:]...)
		select {
		case f.outbound <- pkt:
		case <-f.closed:
			return 0, os.ErrClosed
		}
	}
	return len(bufs), nil
}

func (f *fakeTUN) MTU() (int, error)        { return 1280, nil }
func (f *fakeTUN) Name() (string, error)    { return "fake0", nil }
func (f *fakeTUN) File() *os.File           { return nil }
func (f *fakeTUN) Events() <-chan tun.Event { return f.events }
func (f *fakeTUN) BatchSize() int           { return 1 }
func (f *fakeTUN) Close() error {
	f.closeOnce.Do(func() { close(f.closed) })
	return nil
}

// memDatagrammer is a non-QUIC Datagrammer: a pair of channels. It proves the
// data-plane pump carries no QUIC-specific assumptions.
type memDatagrammer struct {
	out chan []byte // SendDatagram writes here
	in  chan []byte // ReceiveDatagram reads here
}

func (m *memDatagrammer) SendDatagram(b []byte) error {
	m.out <- append([]byte(nil), b...)
	return nil
}

func (m *memDatagrammer) ReceiveDatagram(ctx context.Context) ([]byte, error) {
	select {
	case b := <-m.in:
		return b, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// compile-time check: memDatagrammer (no QUIC) is a valid transport.
var _ Datagrammer = (*memDatagrammer)(nil)

func ipv4Packet(src, dst [4]byte, payload []byte) []byte {
	pkt := make([]byte, 20+len(payload))
	pkt[0] = 0x45
	binary.BigEndian.PutUint16(pkt[2:], uint16(len(pkt)))
	pkt[8] = 64
	pkt[9] = 17 // UDP
	copy(pkt[12:], src[:])
	copy(pkt[16:], dst[:])
	binary.BigEndian.PutUint16(pkt[10:], checksum(pkt[:20]))
	copy(pkt[20:], payload)
	return pkt
}

// TestPumpOverMemTransport runs the Phase-0 pump on both ends connected by a
// non-QUIC Datagrammer and a fake TUN, and checks a packet injected at one end
// arrives intact at the other — i.e. the pump is fully transport-agnostic.
func TestPumpOverMemTransport(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	link1 := make(chan []byte, 16) // A -> B
	link2 := make(chan []byte, 16) // B -> A
	dA := &memDatagrammer{out: link1, in: link2}
	dB := &memDatagrammer{out: link2, in: link1}

	devA, devB := newFakeTUN(), newFakeTUN()
	defer devA.Close()
	defer devB.Close()

	cfg := Config{MaxDatagram: 1300}
	go func() { _ = Run(ctx, devA, dA, cfg) }()
	go func() { _ = Run(ctx, devB, dB, cfg) }()

	want := ipv4Packet([4]byte{10, 7, 0, 2}, [4]byte{10, 7, 0, 1}, []byte("hello-siega"))
	devA.inbound <- want

	select {
	case got := <-devB.outbound:
		if string(got) != string(want) {
			t.Fatalf("packet corrupted:\n got %x\nwant %x", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("packet did not traverse the pump within 2s")
	}
}
