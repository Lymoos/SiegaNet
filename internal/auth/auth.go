// Package auth implements SiegaNet peer authentication.
//
// A client proves possession of its per-peer pre-shared key (PSK) with a
// time-bounded HMAC, sent in an HTTP Authorization header on the WebTransport
// upgrade request:
//
//	Authorization: SiegaNet <peerID>:<token>
//	token = hex( HMAC_SHA256(psk, canonical(peerID, unix_minute)) )
//
// The message is canonically encoded (length-prefixed peerID + fixed-width
// minute) so no two (peerID, minute) pairs can ever alias. The server accepts
// unix_minute in {now-1, now, now+1} to tolerate clock skew, compares in
// constant time, and — crucially for the decoy's indistinguishability — does
// the *same* constant amount of work (three HMACs + a constant-time compare)
// whether the peer is known, unknown, or the header is malformed, so a prober
// cannot learn anything from response timing.
//
// Only the standard library is used for crypto (crypto/hmac, crypto/sha256,
// crypto/subtle), per the project's no-home-grown-crypto rule. The package is
// transport-agnostic: it consumes a header string and a PSK lookup, with no
// dependency on QUIC/WebTransport.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"strings"
	"time"
)

// Scheme is the Authorization scheme name.
const Scheme = "SiegaNet"

// tokenLen is the HMAC-SHA256 output size in bytes.
const tokenLen = sha256.Size

// PeerKeyLookup resolves a peer's PSK. ok=false means the peer is unknown; the
// caller still performs a constant-time comparison against a dummy key so the
// unknown-peer path is indistinguishable from a wrong-token path.
type PeerKeyLookup interface {
	PSK(peerID string) (psk []byte, ok bool)
}

// Authenticator verifies peer tokens against a key lookup.
type Authenticator struct {
	keys     PeerKeyLookup
	now      func() time.Time
	skew     int    // minutes of tolerance on each side
	dummyPSK []byte // used to equalise timing for unknown peers
}

// Option configures an Authenticator.
type Option func(*Authenticator)

// WithClock overrides the time source (for tests).
func WithClock(now func() time.Time) Option { return func(a *Authenticator) { a.now = now } }

// WithSkew sets the per-side minute tolerance (default 1).
func WithSkew(minutes int) Option {
	return func(a *Authenticator) {
		if minutes >= 0 {
			a.skew = minutes
		}
	}
}

// New builds an Authenticator over the given key lookup.
func New(keys PeerKeyLookup, opts ...Option) *Authenticator {
	a := &Authenticator{keys: keys, now: time.Now, skew: 1, dummyPSK: make([]byte, 32)}
	_, _ = rand.Read(a.dummyPSK) // random per-process key for the unknown-peer path
	for _, o := range opts {
		o(a)
	}
	return a
}

// canonical encodes the HMAC message as:
//
//	uint16(len(peerID)) || peerID || uint64(minute)
//
// The length prefix and fixed-width minute make the encoding injective: no two
// distinct (peerID, minute) pairs share an encoding. peerID longer than 65535
// bytes is rejected by the caller.
func canonical(peerID string, minute int64) []byte {
	b := make([]byte, 2+len(peerID)+8)
	binary.BigEndian.PutUint16(b[0:2], uint16(len(peerID)))
	copy(b[2:], peerID)
	binary.BigEndian.PutUint64(b[2+len(peerID):], uint64(minute))
	return b
}

// rawToken computes the HMAC-SHA256 token bytes for (peerID, minute).
func rawToken(psk []byte, peerID string, minute int64) []byte {
	m := hmac.New(sha256.New, psk)
	m.Write(canonical(peerID, minute))
	return m.Sum(nil)
}

// Token returns the hex token a client sends for the given wall-clock time.
func Token(psk []byte, peerID string, t time.Time) string {
	return hex.EncodeToString(rawToken(psk, peerID, t.Unix()/60))
}

// BuildHeader returns the full Authorization header value for a client.
func BuildHeader(psk []byte, peerID string, t time.Time) string {
	return Scheme + " " + peerID + ":" + Token(psk, peerID, t)
}

// Authorize validates an Authorization header value (everything after the
// header name). On success it returns the authenticated peerID. The work done
// is constant regardless of outcome to avoid leaking peer existence via timing.
func (a *Authenticator) Authorize(headerValue string) (peerID string, ok bool) {
	id, tokenRaw, parsed := parse(headerValue)

	// Resolve the key, falling back to the dummy key so the comparison loop runs
	// identically for unknown peers.
	psk, known := a.keys.PSK(id)
	if !known || len(id) > 0xffff {
		psk = a.dummyPSK
	}

	// Always run the same number of HMACs and one constant-time compare.
	minute := a.now().Unix() / 60
	var match int
	for d := -a.skew; d <= a.skew; d++ {
		expected := rawToken(psk, id, minute+int64(d))
		match |= subtle.ConstantTimeCompare(expected, tokenRaw)
	}

	authed := parsed && known && match == 1
	if !authed {
		return "", false
	}
	return id, true
}

// parse splits "SiegaNet <peerID>:<hextoken>" into its parts. tokenRaw is always
// tokenLen bytes (zero-filled on any parse failure) so the caller's compare
// runs in constant time regardless of input shape.
func parse(headerValue string) (peerID string, tokenRaw []byte, ok bool) {
	tokenRaw = make([]byte, tokenLen)

	scheme, rest, found := strings.Cut(headerValue, " ")
	if !found || scheme != Scheme {
		return "", tokenRaw, false
	}
	id, tok, found := strings.Cut(strings.TrimSpace(rest), ":")
	if !found || id == "" {
		return "", tokenRaw, false
	}
	decoded, err := hex.DecodeString(tok)
	if err != nil || len(decoded) != tokenLen {
		// Keep the zero-filled tokenRaw; the compare will fail in constant time.
		return id, tokenRaw, false
	}
	copy(tokenRaw, decoded)
	return id, tokenRaw, true
}
