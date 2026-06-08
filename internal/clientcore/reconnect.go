package clientcore

import (
	"context"
	"errors"
	"sync"
	"time"
)

// errHolderClosed is returned to the data-plane once the supervisor is shutting
// down, which makes tunnel.Run's receive loop exit cleanly.
var errHolderClosed = errors.New("clientcore: session holder closed")

// reconnectingSession is a tunnel.Datagrammer that hides session churn from the
// data-plane. On a session error it requests a redial and blocks the receive
// loop until a fresh session is swapped in, rather than surfacing the error (so
// tunnel.Run keeps running over a single, stable holder). Outbound datagrams
// during the gap are dropped (the inner TCP retransmits).
type reconnectingSession struct {
	dial func(context.Context) (Session, error)
	log  func(string, ...any)

	mu      sync.Mutex
	cond    *sync.Cond
	cur     Session
	gen     uint64 // bumped on every successful swap
	pending bool   // a redial is queued/in-flight
	closed  bool
	reqCh   chan struct{} // capacity 1: "please redial"
}

func newReconnectingSession(first Session, dial func(context.Context) (Session, error), log func(string, ...any)) *reconnectingSession {
	r := &reconnectingSession{dial: dial, log: log, cur: first, gen: 1, reqCh: make(chan struct{}, 1)}
	r.cond = sync.NewCond(&r.mu)
	return r
}

func (r *reconnectingSession) logf(f string, a ...any) {
	if r.log != nil {
		r.log(f, a...)
	}
}

func (r *reconnectingSession) snapshot() (Session, uint64, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cur, r.gen, r.closed
}

// requestRedial signals the redial loop, but only if the broken session is still
// the current one (brokenGen == gen) and no redial is already pending — so
// concurrent Send/Receive failures collapse into a single redial.
func (r *reconnectingSession) requestRedial(brokenGen uint64) {
	r.mu.Lock()
	if !r.closed && brokenGen == r.gen && !r.pending {
		r.pending = true
		select {
		case r.reqCh <- struct{}{}:
		default:
		}
	}
	r.mu.Unlock()
}

// waitNewer blocks until a session newer than gen g is available, or the holder
// closes. Returns ok=false when closed.
func (r *reconnectingSession) waitNewer(g uint64) (Session, uint64, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for r.gen <= g && !r.closed {
		r.cond.Wait()
	}
	if r.closed {
		return nil, 0, false
	}
	return r.cur, r.gen, true
}

// ReceiveDatagram delegates to the current session, re-dialing and waiting on a
// break instead of returning the error to the data-plane.
func (r *reconnectingSession) ReceiveDatagram(ctx context.Context) ([]byte, error) {
	s, g, closed := r.snapshot()
	for {
		if closed {
			return nil, errHolderClosed
		}
		b, err := s.ReceiveDatagram(ctx)
		if err == nil {
			return b, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		r.requestRedial(g)
		var ok bool
		if s, g, ok = r.waitNewer(g); !ok {
			return nil, errHolderClosed
		}
	}
}

// SendDatagram sends on the current session; on failure it triggers a redial and
// drops the packet (never blocks the outbound pump).
func (r *reconnectingSession) SendDatagram(b []byte) error {
	s, g, closed := r.snapshot()
	if closed {
		return errHolderClosed
	}
	if err := s.SendDatagram(b); err != nil {
		r.requestRedial(g)
	}
	return nil
}

// redialLoop services redial requests until ctx is cancelled.
func (r *reconnectingSession) redialLoop(ctx context.Context) {
	b := backoff{min: time.Second, max: 30 * time.Second}
	for {
		select {
		case <-ctx.Done():
			return
		case <-r.reqCh:
		}

		old, _, _ := r.snapshot()
		var ns Session
		for {
			s, err := r.dial(ctx)
			if err == nil {
				ns = s
				break
			}
			if ctx.Err() != nil {
				return
			}
			r.logf("reconnect dial failed (%v); retrying", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(b.next()):
			}
		}
		b.reset()
		if old != nil {
			_ = old.Close()
		}
		r.mu.Lock()
		r.cur = ns
		r.gen++
		r.pending = false
		gen := r.gen
		r.mu.Unlock()
		r.cond.Broadcast()
		r.logf("reconnected (session #%d)", gen)
	}
}

// close shuts the holder down, unblocking any waiting receivers and closing the
// current session.
func (r *reconnectingSession) close() {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	cur := r.cur
	r.mu.Unlock()
	r.cond.Broadcast()
	if cur != nil {
		_ = cur.Close()
	}
}
