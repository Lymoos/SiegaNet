package router

import (
	"context"
	"encoding/binary"
	"errors"
	"net/netip"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.zx2c4.com/wireguard/tun"

	"github.com/lymoos/sieganet/internal/protocol"
	"github.com/lymoos/sieganet/internal/tundev"
)

var errSessClosed = errors.New("session closed")

// memSession is an in-memory Datagrammer. in feeds ReceiveDatagram (peer→server);
// out captures SendDatagram (server→peer). abrupt() simulates a lost connection.
type memSession struct {
	in     chan []byte
	out    chan []byte
	closed chan struct{}
	once   sync.Once
}

func newMemSession() *memSession {
	return &memSession{in: make(chan []byte, 16), out: make(chan []byte, 16), closed: make(chan struct{})}
}

func (m *memSession) SendDatagram(b []byte) error {
	select {
	case m.out <- append([]byte(nil), b...):
		return nil
	case <-m.closed:
		return errSessClosed
	}
}

func (m *memSession) ReceiveDatagram(ctx context.Context) ([]byte, error) {
	select {
	case b := <-m.in:
		return b, nil
	case <-m.closed:
		return nil, errSessClosed
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (m *memSession) abrupt() { m.once.Do(func() { close(m.closed) }) }

// recvPacket decodes the next datagram the server sent to this peer, or fails.
func (m *memSession) recvPacket(t *testing.T) []byte {
	t.Helper()
	select {
	case dg := <-m.out:
		pkt, err := protocol.DecodeDatagram(dg)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		return pkt
	case <-time.After(time.Second):
		t.Fatal("expected a packet to the peer, got none")
		return nil
	}
}

func (m *memSession) expectNoPacket(t *testing.T) {
	t.Helper()
	select {
	case <-m.out:
		t.Fatal("peer received a packet it should not have")
	case <-time.After(150 * time.Millisecond):
	}
}

// ---- fake TUN ----

type fakeTUN struct {
	inbound  chan []byte
	outbound chan []byte
	closed   chan struct{}
	once     sync.Once
	events   chan tun.Event
}

func newFakeTUN() *fakeTUN {
	return &fakeTUN{inbound: make(chan []byte, 16), outbound: make(chan []byte, 16), closed: make(chan struct{}), events: make(chan tun.Event)}
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
		select {
		case f.outbound <- append([]byte(nil), b[offset:]...):
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
func (f *fakeTUN) Close() error             { f.once.Do(func() { close(f.closed) }); return nil }

// ---- accounter ----

type accounter struct{ open, closed atomic.Int64 }

func (a *accounter) SessionOpened(string) int { a.open.Add(1); return 0 }
func (a *accounter) SessionClosed(string) int { a.closed.Add(1); return 0 }

// ---- helpers ----

func ipv4(src, dst string, payload []byte) []byte {
	s := netip.MustParseAddr(src).As4()
	d := netip.MustParseAddr(dst).As4()
	pkt := make([]byte, 20+len(payload))
	pkt[0] = 0x45
	binary.BigEndian.PutUint16(pkt[2:], uint16(len(pkt)))
	pkt[8] = 64
	pkt[9] = 17
	copy(pkt[12:], s[:])
	copy(pkt[16:], d[:])
	copy(pkt[20:], payload)
	return pkt
}

func feed(t *testing.T, m *memSession, pkt []byte) {
	t.Helper()
	dg, err := protocol.EncodeDatagram(nil, pkt, protocol.PadRange{})
	if err != nil {
		t.Fatal(err)
	}
	m.in <- dg
}

const (
	serverIP = "10.7.0.1"
	ipA      = "10.7.0.2"
	ipB      = "10.7.0.3"
)

func newRouter(t *testing.T, allowC2C bool) (*Router, *fakeTUN, *accounter) {
	dev := newFakeTUN()
	acc := &accounter{}
	r := New(dev, netip.MustParseAddr(serverIP), netip.MustParsePrefix("10.7.0.0/24"), allowC2C, protocol.PadRange{}, acc)
	t.Cleanup(func() { dev.Close() })
	return r, dev, acc
}

func serve(ctx context.Context, r *Router, peerID, ip string, m *memSession) {
	go r.Serve(ctx, peerID, netip.MustParseAddr(ip), m)
}

func TestAntiSpoofIsolation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r, dev, _ := newRouter(t, true)
	a, b := newMemSession(), newMemSession()
	serve(ctx, r, "A", ipA, a)
	serve(ctx, r, "B", ipB, b)
	time.Sleep(50 * time.Millisecond) // let sessions register

	// A spoofs B's source address. Must be dropped, never delivered to B or TUN.
	feed(t, a, ipv4(ipB, "8.8.8.8", []byte("spoofed")))
	b.expectNoPacket(t)
	select {
	case <-dev.outbound:
		t.Fatal("spoofed packet reached the TUN")
	case <-time.After(150 * time.Millisecond):
	}
	if got := r.Stats.SpoofDrops.Load(); got != 1 {
		t.Fatalf("SpoofDrops=%d, want 1", got)
	}

	// A's legitimate packet to the internet reaches the TUN.
	feed(t, a, ipv4(ipA, "8.8.8.8", []byte("legit")))
	select {
	case <-dev.outbound:
	case <-time.After(time.Second):
		t.Fatal("legit packet did not reach the TUN")
	}
}

func TestInterClientDisabledByDefault(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r, _, _ := newRouter(t, false) // allow_inter_client = false
	a, b := newMemSession(), newMemSession()
	serve(ctx, r, "A", ipA, a)
	serve(ctx, r, "B", ipB, b)
	time.Sleep(50 * time.Millisecond)

	feed(t, a, ipv4(ipA, ipB, []byte("hi B")))
	b.expectNoPacket(t)
	if got := r.Stats.C2CDrops.Load(); got != 1 {
		t.Fatalf("C2CDrops=%d, want 1", got)
	}
}

func TestInterClientEnabledRoutes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r, _, _ := newRouter(t, true) // allow_inter_client = true
	a, b := newMemSession(), newMemSession()
	serve(ctx, r, "A", ipA, a)
	serve(ctx, r, "B", ipB, b)
	time.Sleep(50 * time.Millisecond)

	payload := []byte("hi B from A")
	feed(t, a, ipv4(ipA, ipB, payload))
	got := b.recvPacket(t)
	if string(got[20:]) != string(payload) {
		t.Fatalf("B got %q, want %q", got[20:], payload)
	}
}

func TestSessionCounterDecrementsOnAbruptDisconnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r, _, acc := newRouter(t, true)
	a := newMemSession()
	serve(ctx, r, "A", ipA, a)
	time.Sleep(50 * time.Millisecond)
	if acc.open.Load() != 1 || r.ActivePeers() != 1 {
		t.Fatalf("session not registered: open=%d active=%d", acc.open.Load(), r.ActivePeers())
	}

	a.abrupt() // simulate a lost connection (not a graceful close)

	deadline := time.After(2 * time.Second)
	for {
		if acc.closed.Load() == 1 && r.ActivePeers() == 0 {
			return // counter decremented, session unregistered
		}
		select {
		case <-deadline:
			t.Fatalf("counter leaked: closed=%d active=%d", acc.closed.Load(), r.ActivePeers())
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func TestOutboundDemuxFromTUN(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r, dev, _ := newRouter(t, true)
	a := newMemSession()
	serve(ctx, r, "A", ipA, a)
	time.Sleep(50 * time.Millisecond)
	go func() { _ = r.Run(ctx) }()

	// Return traffic from the internet to A arrives on the TUN.
	dev.inbound <- ipv4("8.8.8.8", ipA, []byte("reply"))
	got := a.recvPacket(t)
	if string(got[20:]) != "reply" {
		t.Fatalf("A got %q, want reply", got[20:])
	}

	// Traffic for an unknown inner IP is dropped (no session).
	dev.inbound <- ipv4("8.8.8.8", "10.7.0.99", []byte("nobody"))
	deadline := time.After(time.Second)
	for r.Stats.NoRouteDrops.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("NoRouteDrops not incremented for unknown destination")
		case <-time.After(20 * time.Millisecond):
		}
	}
	_ = tundev.Offset
}
