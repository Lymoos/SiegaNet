package auth

import (
	"testing"
	"time"
)

// mapKeys is a mock PeerKeyLookup. Auth is transport-agnostic, so a plain map is
// all it needs — no QUIC/WebTransport involved.
type mapKeys map[string][]byte

func (m mapKeys) PSK(id string) ([]byte, bool) { k, ok := m[id]; return k, ok }

func atMinute(m int64) time.Time { return time.Unix(m*60, 0) }

func newAuth(keys mapKeys, nowMinute int64) *Authenticator {
	return New(keys, WithClock(func() time.Time { return atMinute(nowMinute) }))
}

func TestAcceptsWithinWindow(t *testing.T) {
	psk := []byte("0123456789abcdef0123456789abcdef")
	keys := mapKeys{"kristina": psk}
	a := newAuth(keys, 1000)

	for _, m := range []int64{999, 1000, 1001} {
		hdr := BuildHeader(psk, "kristina", atMinute(m))
		if id, ok := a.Authorize(hdr); !ok || id != "kristina" {
			t.Errorf("minute %d: got (%q,%v), want (kristina,true)", m, id, ok)
		}
	}
}

func TestRejectsOutsideWindow(t *testing.T) {
	psk := []byte("0123456789abcdef0123456789abcdef")
	a := newAuth(mapKeys{"kristina": psk}, 1000)
	for _, m := range []int64{998, 1002, 0, 5000} {
		hdr := BuildHeader(psk, "kristina", atMinute(m))
		if _, ok := a.Authorize(hdr); ok {
			t.Errorf("minute %d: accepted a token outside the ±1 window", m)
		}
	}
}

func TestUnknownPeerRejected(t *testing.T) {
	psk := []byte("0123456789abcdef0123456789abcdef")
	a := newAuth(mapKeys{"kristina": psk}, 1000)
	hdr := BuildHeader(psk, "mallory", atMinute(1000)) // valid HMAC, unknown peer
	if _, ok := a.Authorize(hdr); ok {
		t.Error("unknown peer accepted")
	}
}

func TestWrongPSKRejected(t *testing.T) {
	good := []byte("0123456789abcdef0123456789abcdef")
	bad := []byte("ffffffffffffffffffffffffffffffff")
	a := newAuth(mapKeys{"kristina": good}, 1000)
	hdr := BuildHeader(bad, "kristina", atMinute(1000))
	if _, ok := a.Authorize(hdr); ok {
		t.Error("wrong PSK accepted")
	}
}

func TestMalformedHeaders(t *testing.T) {
	psk := []byte("0123456789abcdef0123456789abcdef")
	a := newAuth(mapKeys{"kristina": psk}, 1000)
	valid := Token(psk, "kristina", atMinute(1000))
	cases := []string{
		"",                               // empty
		"kristina:" + valid,              // no scheme
		"Bearer kristina:" + valid,       // wrong scheme
		"SiegaNet kristina",              // no colon
		"SiegaNet :" + valid,             // empty peerID
		"SiegaNet kristina:zzzz",         // non-hex token
		"SiegaNet kristina:abcd",         // wrong length token
		"SiegaNet kristina:" + valid[2:], // truncated hex
	}
	for _, h := range cases {
		if _, ok := a.Authorize(h); ok {
			t.Errorf("malformed header accepted: %q", h)
		}
	}
}

// TestCanonicalEncodingNoAlias documents that the length-prefixed, fixed-width
// encoding prevents (peerID, minute) aliasing that a naive concatenation
// (peerID || itoa(minute)) would allow: "user1"@minute2 vs "user"@minute12
// would both naively serialise to "user12".
func TestCanonicalEncodingNoAlias(t *testing.T) {
	psk := []byte("0123456789abcdef0123456789abcdef")
	a := Token(psk, "user1", atMinute(2))
	b := Token(psk, "user", atMinute(12))
	if a == b {
		t.Fatal("tokens aliased across different (peerID, minute) pairs")
	}
}

// TestUnknownAndWrongBothFail is a behavioural stand-in for the constant-time
// property (true timing parity is verified at the HTTP layer in step 4): the
// unknown-peer path and the wrong-token path both reject.
func TestUnknownAndWrongBothFail(t *testing.T) {
	psk := []byte("0123456789abcdef0123456789abcdef")
	a := newAuth(mapKeys{"kristina": psk}, 1000)
	if _, ok := a.Authorize(BuildHeader(psk, "ghost", atMinute(1000))); ok {
		t.Error("unknown peer accepted")
	}
	wrong := "SiegaNet kristina:" + Token([]byte("wrongwrongwrongwrongwrongwrong12"), "kristina", atMinute(1000))
	if _, ok := a.Authorize(wrong); ok {
		t.Error("wrong token accepted")
	}
}
