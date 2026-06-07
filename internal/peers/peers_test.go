package peers_test

import (
	"bytes"
	"net/netip"
	"path/filepath"
	"testing"
	"time"

	"github.com/lymoos/sieganet/internal/auth"
	"github.com/lymoos/sieganet/internal/peers"
)

func newStore(t *testing.T, cidr string) (*peers.FileStore, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "peers.toml")
	s, err := peers.NewFileStore(path, netip.MustParsePrefix(cidr), firstHostOf(cidr))
	if err != nil {
		t.Fatal(err)
	}
	return s, path
}

// firstHostOf returns network+1 as the reserved server IP.
func firstHostOf(cidr string) netip.Addr {
	return netip.MustParsePrefix(cidr).Masked().Addr().Next()
}

func TestAddAllocatesSequentialIPsSkippingServer(t *testing.T) {
	s, _ := newStore(t, "10.7.0.0/24") // server reserved = 10.7.0.1
	for _, want := range []string{"10.7.0.2", "10.7.0.3", "10.7.0.4"} {
		p, err := s.Add("peer-" + want)
		if err != nil {
			t.Fatal(err)
		}
		if p.InnerIP.String() != want {
			t.Errorf("got %s, want %s", p.InnerIP, want)
		}
	}
}

func TestPSKIsPerPeerAndStrong(t *testing.T) {
	s, _ := newStore(t, "10.7.0.0/24")
	a, _ := s.Add("a")
	b, _ := s.Add("b")
	if bytes.Equal(a.PSK, b.PSK) {
		t.Fatal("two peers share a PSK — must be per-peer")
	}
	if len(a.PSK) != 32 {
		t.Fatalf("PSK length %d, want 32", len(a.PSK))
	}
}

func TestStableAcrossRestart(t *testing.T) {
	s, path := newStore(t, "10.7.0.0/24")
	a, _ := s.Add("kristina")
	b, _ := s.Add("dmitry")

	// Reopen the same file in a fresh store.
	s2, err := peers.NewFileStore(path, netip.MustParsePrefix("10.7.0.0/24"), firstHostOf("10.7.0.0/24"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []peers.Peer{a, b} {
		got, ok := s2.Lookup(want.ID)
		if !ok {
			t.Fatalf("peer %s missing after restart", want.ID)
		}
		if got.InnerIP != want.InnerIP {
			t.Errorf("%s IP changed across restart: %s -> %s", want.ID, want.InnerIP, got.InnerIP)
		}
		if !bytes.Equal(got.PSK, want.PSK) {
			t.Errorf("%s PSK changed across restart", want.ID)
		}
	}
}

func TestRemoveFreesAndLowestReused(t *testing.T) {
	s, _ := newStore(t, "10.7.0.0/24")
	a, _ := s.Add("a") // .2
	_, _ = s.Add("b")  // .3
	if err := s.Remove("a"); err != nil {
		t.Fatal(err)
	}
	c, _ := s.Add("c") // should reuse the lowest free = .2
	if c.InnerIP != a.InnerIP {
		t.Errorf("freed IP not reused: got %s want %s", c.InnerIP, a.InnerIP)
	}
	if _, ok := s.Lookup("a"); ok {
		t.Error("removed peer still present")
	}
}

func TestPoolExhaustion(t *testing.T) {
	// /30 => hosts .1 (server, reserved) and .2; only one peer fits.
	s, _ := newStore(t, "10.7.0.0/30")
	if _, err := s.Add("a"); err != nil {
		t.Fatalf("first add: %v", err)
	}
	if _, err := s.Add("b"); err != peers.ErrPoolExhausted {
		t.Fatalf("expected ErrPoolExhausted, got %v", err)
	}
}

func TestRevokeBlocksAuthButKeepsRecord(t *testing.T) {
	s, _ := newStore(t, "10.7.0.0/24")
	a, _ := s.Add("a")
	if _, ok := s.PSK("a"); !ok {
		t.Fatal("PSK missing before revoke")
	}
	if err := s.Revoke("a"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.PSK("a"); ok {
		t.Error("revoked peer still authenticates")
	}
	got, ok := s.Lookup("a")
	if !ok || !got.Revoked || got.InnerIP != a.InnerIP {
		t.Error("revoked peer record/IP not preserved")
	}
}

func TestRotatePSKChangesKeyAndClearsRevoke(t *testing.T) {
	s, _ := newStore(t, "10.7.0.0/24")
	a, _ := s.Add("a")
	_ = s.Revoke("a")
	rotated, err := s.RotatePSK("a")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(rotated.PSK, a.PSK) {
		t.Error("rotate produced the same PSK")
	}
	if rotated.Revoked {
		t.Error("rotate did not clear revoked")
	}
	if _, ok := s.PSK("a"); !ok {
		t.Error("PSK refused after rotate (should authenticate)")
	}
}

func TestSessionAccounting(t *testing.T) {
	s, _ := newStore(t, "10.7.0.0/24")
	_, _ = s.Add("a")
	if n := s.SessionOpened("a"); n != 1 {
		t.Fatalf("opened=1 expected, got %d", n)
	}
	if n := s.SessionOpened("a"); n != 2 {
		t.Fatalf("opened=2 expected, got %d", n)
	}
	if n := s.ActiveSessions("a"); n != 2 {
		t.Fatalf("active=2 expected, got %d", n)
	}
	s.SessionClosed("a")
	if n := s.ActiveSessions("a"); n != 1 {
		t.Fatalf("active=1 expected, got %d", n)
	}
	// never goes negative
	s.SessionClosed("a")
	s.SessionClosed("a")
	if n := s.ActiveSessions("a"); n != 0 {
		t.Fatalf("active=0 expected, got %d", n)
	}
}

func TestLookupReturnsCopy(t *testing.T) {
	s, _ := newStore(t, "10.7.0.0/24")
	_, _ = s.Add("a")
	got, _ := s.Lookup("a")
	got.PSK[0] ^= 0xff // mutate the returned copy
	again, _ := s.Lookup("a")
	if again.PSK[0] == got.PSK[0] {
		t.Error("store leaked a mutable PSK reference")
	}
}

// TestStoreSatisfiesAuthThroughInterface exercises the store only through the
// auth.PeerKeyLookup interface (a real consumer), proving revocation takes
// effect at the auth layer.
func TestStoreSatisfiesAuthThroughInterface(t *testing.T) {
	s, _ := newStore(t, "10.7.0.0/24")
	p, _ := s.Add("kristina")

	var lookup auth.PeerKeyLookup = s // interface, not concrete type
	a := auth.New(lookup)

	hdr := auth.BuildHeader(p.PSK, "kristina", time.Now())
	if _, ok := a.Authorize(hdr); !ok {
		t.Fatal("valid peer rejected via store-backed auth")
	}
	_ = s.Revoke("kristina")
	if _, ok := a.Authorize(hdr); ok {
		t.Fatal("revoked peer still authenticates")
	}
}

func TestClientConfigRoundTrip(t *testing.T) {
	s, _ := newStore(t, "10.7.0.0/24")
	p, _ := s.Add("kristina")
	cc := peers.NewClientConfig(p, "example.com:443", "/wt/secret", "1.1.1.1", 1280)

	got, err := peers.ParseURL(cc.URL())
	if err != nil {
		t.Fatal(err)
	}
	if got != cc {
		t.Errorf("round-trip mismatch:\n got %+v\nwant %+v", got, cc)
	}
	if !bytes.Contains([]byte(cc.TOML()), []byte(`peer_id     = "kristina"`)) {
		t.Errorf("TOML missing peer_id:\n%s", cc.TOML())
	}
}
