// Package peers manages SiegaNet peers: their per-peer pre-shared key, their
// stable inner-subnet IP, revocation/rotation, and runtime session accounting.
//
// All consumers (the server, the session router, sieganet-ctl) go through the
// Store interface; the concrete implementation is a local TOML file
// (store_file.go), but a multi-node backend can replace it without touching
// callers. There is deliberately NO shared-key mode: every peer has its own PSK.
package peers

import (
	"errors"
	"net/netip"
	"time"
)

// Errors returned by a Store.
var (
	ErrNotFound      = errors.New("peers: peer not found")
	ErrExists        = errors.New("peers: peer already exists")
	ErrPoolExhausted = errors.New("peers: inner subnet has no free addresses")
	ErrRevoked       = errors.New("peers: peer is revoked")
)

// Peer is the persisted model for one peer.
type Peer struct {
	ID      string     // peerID
	PSK     []byte     // per-peer pre-shared key (CSPRNG, never shared)
	InnerIP netip.Addr // stable inner-subnet address
	Revoked bool
	Created time.Time
}

// clone returns a deep copy so callers cannot mutate the store's state.
func (p Peer) clone() Peer {
	cp := p
	cp.PSK = append([]byte(nil), p.PSK...)
	return cp
}

// Store is the peer backend. Persistent CRUD plus security operations (revoke,
// rotate) and runtime, non-persisted session accounting that the router updates.
type Store interface {
	// Lookup returns a copy of the peer and whether it exists.
	Lookup(peerID string) (Peer, bool)

	// Add creates a peer with a fresh CSPRNG PSK and the next free inner IP.
	Add(peerID string) (Peer, error)

	// Remove deletes a peer and frees its inner IP.
	Remove(peerID string) error

	// List returns copies of all peers, ordered by inner IP.
	List() []Peer

	// Revoke marks a peer revoked: PSK() then refuses it, so authentication
	// fails immediately without deleting the record (IP stays reserved).
	Revoke(peerID string) error

	// RotatePSK replaces a peer's PSK with a fresh CSPRNG key and returns the
	// updated peer. Also clears the revoked flag.
	RotatePSK(peerID string) (Peer, error)

	// PSK implements auth.PeerKeyLookup: it returns the key only for an existing,
	// non-revoked peer (ok=false otherwise), so revocation takes effect at once.
	PSK(peerID string) (psk []byte, ok bool)

	// Session accounting (runtime only, not persisted). The router increments on
	// session open and decrements on close; a future per-peer device/IP limit and
	// auto-ban build on ActiveSessions.
	SessionOpened(peerID string) int
	SessionClosed(peerID string) int
	ActiveSessions(peerID string) int
}

// pskLen is the per-peer key length in bytes.
const pskLen = 32

// allocator hands out the lowest free host address in a prefix, skipping the
// network address, the broadcast address and the reserved server IP.
type allocator struct {
	subnet   netip.Prefix
	serverIP netip.Addr
}

// firstHost returns the first usable host address (network+1).
func (a allocator) firstHost() netip.Addr { return a.subnet.Masked().Addr().Next() }

// broadcast returns the last address of the (IPv4) prefix.
func (a allocator) broadcast() netip.Addr {
	v4 := a.subnet.Masked().Addr().As4()
	host := uint32(1)<<(32-a.subnet.Bits()) - 1
	n := uint32(v4[0])<<24 | uint32(v4[1])<<16 | uint32(v4[2])<<8 | uint32(v4[3])
	n |= host
	return netip.AddrFrom4([4]byte{byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)})
}

// next returns the lowest address in the prefix not present in used and not
// reserved. Deterministic given the current used set, so the same peers always
// produce the same assignments after a restart.
func (a allocator) next(used map[netip.Addr]bool) (netip.Addr, error) {
	bcast := a.broadcast()
	for addr := a.firstHost(); a.subnet.Contains(addr); addr = addr.Next() {
		if addr == bcast || addr == a.serverIP || used[addr] {
			continue
		}
		return addr, nil
	}
	return netip.Addr{}, ErrPoolExhausted
}
