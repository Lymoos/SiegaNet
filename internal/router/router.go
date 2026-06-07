// Package router is the server-side session multiplexer. Unlike the Phase-0 pump
// (one TUN ↔ one connection), the server owns a single TUN and many peer
// sessions, and routes IP packets between them by inner-subnet address.
//
// Two invariants matter for security:
//
//   - Anti-spoofing / peer isolation: a packet received from a peer's session is
//     accepted only if its source IP equals the inner IP assigned to that peer.
//     A peer can therefore never impersonate another peer (or the server).
//   - Inter-client policy: a packet addressed to another peer's inner IP is
//     forwarded peer-to-peer only when allow_inter_client is enabled; otherwise
//     it is dropped, so peers cannot reach each other (or even see that others
//     exist) by default.
//
// Per-peer session accounting is updated through the SessionAccounter interface:
// open increments, and ANY session end (graceful close or abrupt disconnect)
// decrements, because the receive loop exits when the transport dies.
package router

import (
	"context"
	"net/netip"
	"sync"
	"sync/atomic"

	"golang.zx2c4.com/wireguard/tun"

	"github.com/lymoos/sieganet/internal/protocol"
	"github.com/lymoos/sieganet/internal/tundev"
	"github.com/lymoos/sieganet/internal/tunnel"
)

const maxPacket = 65535

// SessionAccounter is the subset of peers.Store the router needs for per-peer
// active-session counting.
type SessionAccounter interface {
	SessionOpened(peerID string) int
	SessionClosed(peerID string) int
}

// Stats are the router's drop counters.
type Stats struct {
	SpoofDrops   atomic.Uint64 // src IP != peer's assigned IP (spoof / isolation)
	C2CDrops     atomic.Uint64 // inter-client packet dropped (policy disabled)
	NoRouteDrops atomic.Uint64 // no session for the destination IP
	SendErrors   atomic.Uint64 // SendDatagram failed (incl. oversize)
	Malformed    atomic.Uint64 // undecodable datagram / non-IPv4
}

type session struct {
	peerID  string
	innerIP netip.Addr
	dg      tunnel.Datagrammer
}

// Router multiplexes the server TUN across peer sessions.
type Router struct {
	dev       tun.Device
	serverIP  netip.Addr
	subnet    netip.Prefix
	allowC2C  bool
	pad       protocol.PadRange
	accounter SessionAccounter

	mu     sync.RWMutex
	writeM sync.Mutex // serialises TUN writes from many session goroutines
	byIP   map[netip.Addr]*session

	Stats Stats
}

// New creates a router over dev. serverIP/subnet define the inner addressing.
func New(dev tun.Device, serverIP netip.Addr, subnet netip.Prefix, allowInterClient bool, pad protocol.PadRange, accounter SessionAccounter) *Router {
	return &Router{
		dev:       dev,
		serverIP:  serverIP,
		subnet:    subnet.Masked(),
		allowC2C:  allowInterClient,
		pad:       pad,
		accounter: accounter,
		byIP:      map[netip.Addr]*session{},
	}
}

// Run reads packets from the TUN (return traffic destined to peers) and demuxes
// each to the owning session. Blocks until ctx is done or the TUN closes.
func (r *Router) Run(ctx context.Context) error {
	batch := r.dev.BatchSize()
	bufs := make([][]byte, batch)
	sizes := make([]int, batch)
	for i := range bufs {
		bufs[i] = make([]byte, tundev.Offset+maxPacket)
	}
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		n, err := r.dev.Read(bufs, sizes, tundev.Offset)
		if err != nil {
			return err
		}
		for i := 0; i < n; i++ {
			pkt := bufs[i][tundev.Offset : tundev.Offset+sizes[i]]
			dst, ok := ipv4Dst(pkt)
			if !ok {
				r.Stats.Malformed.Add(1)
				continue
			}
			if s := r.lookup(dst); s != nil {
				r.sendToPeer(s, pkt)
			} else {
				r.Stats.NoRouteDrops.Add(1)
			}
		}
	}
}

// Serve registers an authenticated peer session and runs its receive loop until
// the session ends, then unregisters it. It is meant to be called from the
// server's OnSession callback and blocks for the session's lifetime.
func (r *Router) Serve(ctx context.Context, peerID string, innerIP netip.Addr, dg tunnel.Datagrammer) {
	s := &session{peerID: peerID, innerIP: innerIP, dg: dg}
	r.register(s)
	if r.accounter != nil {
		r.accounter.SessionOpened(peerID)
	}
	defer func() {
		r.unregister(s)
		if r.accounter != nil {
			r.accounter.SessionClosed(peerID) // runs on graceful close AND abrupt drop
		}
	}()
	r.recvLoop(ctx, s)
}

// recvLoop handles packets coming from a peer: anti-spoof, then route.
func (r *Router) recvLoop(ctx context.Context, s *session) {
	var sendBuf []byte
	for {
		dg, err := s.dg.ReceiveDatagram(ctx)
		if err != nil {
			return // graceful close or abrupt disconnect: loop ends, counter decremented
		}
		pkt, err := protocol.DecodeDatagram(dg)
		if err != nil || len(pkt) == 0 {
			r.Stats.Malformed.Add(1)
			continue
		}
		src, ok := ipv4Src(pkt)
		if !ok {
			r.Stats.Malformed.Add(1)
			continue
		}
		// Anti-spoofing / isolation: the source MUST be this peer's address.
		if src != s.innerIP {
			r.Stats.SpoofDrops.Add(1)
			continue
		}
		dst, ok := ipv4Dst(pkt)
		if !ok {
			r.Stats.Malformed.Add(1)
			continue
		}
		// Destination is another peer (inner subnet, not the server itself).
		if r.subnet.Contains(dst) && dst != r.serverIP {
			if !r.allowC2C {
				r.Stats.C2CDrops.Add(1)
				continue
			}
			if target := r.lookup(dst); target != nil {
				sendBuf = r.sendToPeerBuf(target, pkt, sendBuf)
			} else {
				r.Stats.NoRouteDrops.Add(1)
			}
			continue
		}
		// Destination is the server or the internet: hand to the kernel via TUN.
		r.writeTUN(pkt)
	}
}

func (r *Router) sendToPeer(s *session, pkt []byte) {
	_ = r.sendToPeerBuf(s, pkt, nil)
}

// sendToPeerBuf frames pkt and sends it on the session, reusing buf.
func (r *Router) sendToPeerBuf(s *session, pkt []byte, buf []byte) []byte {
	out, err := protocol.EncodeDatagram(buf, pkt, r.pad)
	if err != nil {
		r.Stats.SendErrors.Add(1)
		return out
	}
	if err := s.dg.SendDatagram(out); err != nil {
		r.Stats.SendErrors.Add(1)
	}
	return out
}

func (r *Router) writeTUN(pkt []byte) {
	buf := make([]byte, tundev.Offset+len(pkt))
	copy(buf[tundev.Offset:], pkt)
	wb := [][]byte{buf}
	r.writeM.Lock()
	_, _ = r.dev.Write(wb, tundev.Offset)
	r.writeM.Unlock()
}

func (r *Router) register(s *session) {
	r.mu.Lock()
	r.byIP[s.innerIP] = s
	r.mu.Unlock()
}

// unregister removes s only if it is still the current session for its IP, so a
// reconnect that replaced the entry is not clobbered by the old session's exit.
func (r *Router) unregister(s *session) {
	r.mu.Lock()
	if cur, ok := r.byIP[s.innerIP]; ok && cur == s {
		delete(r.byIP, s.innerIP)
	}
	r.mu.Unlock()
}

func (r *Router) lookup(ip netip.Addr) *session {
	r.mu.RLock()
	s := r.byIP[ip]
	r.mu.RUnlock()
	return s
}

// ActivePeers returns the number of registered sessions (for diagnostics).
func (r *Router) ActivePeers() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.byIP)
}

func ipv4Src(p []byte) (netip.Addr, bool) {
	if len(p) < 20 || p[0]>>4 != 4 {
		return netip.Addr{}, false
	}
	return netip.AddrFrom4([4]byte{p[12], p[13], p[14], p[15]}), true
}

func ipv4Dst(p []byte) (netip.Addr, bool) {
	if len(p) < 20 || p[0]>>4 != 4 {
		return netip.Addr{}, false
	}
	return netip.AddrFrom4([4]byte{p[16], p[17], p[18], p[19]}), true
}
