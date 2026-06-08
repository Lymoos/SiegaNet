package clientcore

import (
	"context"
	"errors"
	"net/netip"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.zx2c4.com/wireguard/tun"

	"github.com/lymoos/sieganet/internal/tunnel"
)

var errBroken = errors.New("session broke")

type result struct {
	data []byte
	err  error
}

// fakeSession returns scripted results from a channel; an empty channel blocks
// (until ctx) to model a healthy idle session.
type fakeSession struct {
	ch     chan result
	closes atomic.Int32
}

func newFakeSession(buf int) *fakeSession { return &fakeSession{ch: make(chan result, buf)} }

func (f *fakeSession) ReceiveDatagram(ctx context.Context) ([]byte, error) {
	select {
	case r := <-f.ch:
		return r.data, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func (f *fakeSession) SendDatagram([]byte) error { return nil }
func (f *fakeSession) Close() error              { f.closes.Add(1); return nil }

// TestReconnectingSessionContinuity proves the data-plane sees a continuous
// stream across a session break: no error is surfaced, data from the old and the
// new session both arrive.
func TestReconnectingSessionContinuity(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s1 := newFakeSession(2)
	s1.ch <- result{data: []byte("a")}
	s1.ch <- result{err: errBroken}
	s2 := newFakeSession(2)
	s2.ch <- result{data: []byte("b")}

	dialed := make(chan Session, 1)
	dialed <- s2
	dial := func(ctx context.Context) (Session, error) {
		select {
		case s := <-dialed:
			return s, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	r := newReconnectingSession(s1, dial, nil)
	go r.redialLoop(ctx)
	defer r.close()

	got1, err := r.ReceiveDatagram(ctx)
	if err != nil || string(got1) != "a" {
		t.Fatalf("first recv: %q %v", got1, err)
	}
	// The next receive spans the break: s1 errors, the holder re-dials s2 and
	// returns s2's datagram without ever surfacing an error.
	got2, err := r.ReceiveDatagram(ctx)
	if err != nil {
		t.Fatalf("recv across reconnect returned error: %v", err)
	}
	if string(got2) != "b" {
		t.Fatalf("after reconnect got %q, want b", got2)
	}
	// s1 must have been closed when replaced.
	deadline := time.After(time.Second)
	for s1.closes.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("old session not closed on reconnect")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestSendDropsAndTriggersRedialDuringGap(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bad := &errSession{}
	good := newFakeSession(1)
	dialed := make(chan Session, 1)
	dialed <- good
	var redialed atomic.Bool
	dial := func(ctx context.Context) (Session, error) {
		redialed.Store(true)
		select {
		case s := <-dialed:
			return s, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	r := newReconnectingSession(bad, dial, nil)
	go r.redialLoop(ctx)
	defer r.close()

	// Send must never block or error, even though the session is broken.
	if err := r.SendDatagram([]byte("x")); err != nil {
		t.Fatalf("SendDatagram returned error during gap: %v", err)
	}
	deadline := time.After(time.Second)
	for !redialed.Load() {
		select {
		case <-deadline:
			t.Fatal("send failure did not trigger a redial")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

type errSession struct{}

func (errSession) ReceiveDatagram(ctx context.Context) ([]byte, error) { return nil, errBroken }
func (errSession) SendDatagram([]byte) error                           { return errBroken }
func (errSession) Close() error                                        { return nil }

func TestHolderCloseUnblocksReceiver(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	broken := newFakeSession(1)
	broken.ch <- result{err: errBroken}
	// dial blocks forever, so the receiver is stuck waiting for a new session.
	dial := func(ctx context.Context) (Session, error) { <-ctx.Done(); return nil, ctx.Err() }
	r := newReconnectingSession(broken, dial, nil)
	go r.redialLoop(ctx)

	done := make(chan error, 1)
	go func() { _, err := r.ReceiveDatagram(ctx); done <- err }()
	time.Sleep(50 * time.Millisecond) // let it reach waitNewer
	r.close()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected an error after close")
		}
	case <-time.After(time.Second):
		t.Fatal("receiver not unblocked by close")
	}
}

func TestBackoffGrowsAndCaps(t *testing.T) {
	b := backoff{min: time.Second, max: 8 * time.Second}
	var last time.Duration
	for i := 0; i < 10; i++ {
		d := b.next()
		if d <= 0 || d > b.max+b.max/5 {
			t.Fatalf("backoff %v out of bounds", d)
		}
		last = d
	}
	if last < b.max-b.max/5 {
		t.Fatalf("backoff did not reach the cap, last=%v", last)
	}
	b.reset()
	if b.cur != 0 {
		t.Fatal("reset did not clear backoff")
	}
}

// ---- supervisor smoke test with fakes (OS-neutral) ----

type recordingConf struct {
	mu     sync.Mutex
	events []string
}

func (c *recordingConf) record(e string) { c.mu.Lock(); c.events = append(c.events, e); c.mu.Unlock() }
func (c *recordingConf) Sweep()          { c.record("sweep") }
func (c *recordingConf) EngageKillSwitch(netip.Addr, string) error {
	c.record("killswitch")
	return nil
}
func (c *recordingConf) ConfigureTUN(tun.Device, TUNParams) error { c.record("configure"); return nil }
func (c *recordingConf) Cleanup()                                 { c.record("cleanup") }
func (c *recordingConf) has(e string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, x := range c.events {
		if x == e {
			return true
		}
	}
	return false
}

func TestSupervisorRunOrderAndCleanup(t *testing.T) {
	conf := &recordingConf{}
	dialed := atomic.Int32{}
	opt := Options{
		Configurator: conf,
		KillSwitch:   true,
		Params:       TUNParams{TUNName: "siega0", InnerIP: netip.MustParseAddr("10.7.0.2"), InnerMTU: 1280, EndpointAddr: netip.MustParseAddr("203.0.113.1"), EndpointPort: "443"},
		OpenTUN:      func(string, int) (tun.Device, error) { return newSmokeTUN(), nil },
		MaxDatagram:  func(Session) int { return 1300 },
		Dial: func(ctx context.Context) (Session, error) {
			// First dial must happen only after sweep + killswitch.
			if !conf.has("sweep") || !conf.has("killswitch") {
				t.Error("dialed before sweep/killswitch")
			}
			dialed.Add(1)
			return newFakeSession(0), nil // healthy, blocks on receive
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- opt.Run(ctx) }()

	// Let it set up, then shut down.
	deadline := time.After(2 * time.Second)
	for !conf.has("configure") {
		select {
		case <-deadline:
			t.Fatal("never configured the TUN")
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
	for _, e := range []string{"sweep", "killswitch", "configure", "cleanup"} {
		if !conf.has(e) {
			t.Errorf("missing lifecycle event %q (events=%v)", e, conf.events)
		}
	}
}

// smokeTUN is a minimal tun.Device whose Read blocks until close.
type smokeTUN struct {
	closed chan struct{}
	once   sync.Once
	events chan tun.Event
}

func newSmokeTUN() *smokeTUN {
	return &smokeTUN{closed: make(chan struct{}), events: make(chan tun.Event)}
}
func (s *smokeTUN) Read(_ [][]byte, _ []int, _ int) (int, error) { <-s.closed; return 0, os.ErrClosed }
func (s *smokeTUN) Write(b [][]byte, _ int) (int, error)         { return len(b), nil }
func (s *smokeTUN) MTU() (int, error)                            { return 1280, nil }
func (s *smokeTUN) Name() (string, error)                        { return "siega0", nil }
func (s *smokeTUN) File() *os.File                               { return nil }
func (s *smokeTUN) Events() <-chan tun.Event                     { return s.events }
func (s *smokeTUN) BatchSize() int                               { return 1 }
func (s *smokeTUN) Close() error                                 { s.once.Do(func() { close(s.closed) }); return nil }

var _ tunnel.Datagrammer = (*reconnectingSession)(nil)
