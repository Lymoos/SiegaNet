package clientnet

import (
	"bytes"
	"net/netip"
	"testing"
)

func TestCatchAllNRPTFields(t *testing.T) {
	r := catchAllNRPT([]netip.Addr{netip.MustParseAddr("192.0.2.2")})
	if len(r.Names) != 1 || r.Names[0] != "." {
		t.Errorf("Names = %v, want [.]", r.Names)
	}
	if r.ConfigOptions != nrptConfigGenericServers {
		t.Errorf("ConfigOptions = %#x, want %#x", r.ConfigOptions, nrptConfigGenericServers)
	}
	if r.Version != 2 {
		t.Errorf("Version = %d, want 2", r.Version)
	}
	if r.serverList() != "192.0.2.2" {
		t.Errorf("serverList = %q", r.serverList())
	}
}

func TestNRPTServerListJoin(t *testing.T) {
	r := catchAllNRPT([]netip.Addr{netip.MustParseAddr("1.1.1.1"), netip.MustParseAddr("9.9.9.9")})
	if r.serverList() != "1.1.1.1;9.9.9.9" {
		t.Errorf("serverList = %q, want 1.1.1.1;9.9.9.9", r.serverList())
	}
}

// TestNameMultiSZEncoding pins the REG_MULTI_SZ wire format of the "." rule:
// "." (0x2e 0x00) + string terminator (0x00 0x00) + list terminator (0x00 0x00).
func TestNameMultiSZEncoding(t *testing.T) {
	got := catchAllNRPT(nil).nameMultiSZ()
	want := []byte{0x2e, 0x00, 0x00, 0x00, 0x00, 0x00}
	if !bytes.Equal(got, want) {
		t.Errorf("multiSZ(\".\") = % x, want % x", got, want)
	}
}

func TestMultiSZMultiple(t *testing.T) {
	got := multiSZ([]string{"a", "bc"})
	// "a"=61 00, term 00 00; "bc"=62 00 63 00, term 00 00; final 00 00.
	want := []byte{0x61, 0x00, 0x00, 0x00, 0x62, 0x00, 0x63, 0x00, 0x00, 0x00, 0x00, 0x00}
	if !bytes.Equal(got, want) {
		t.Errorf("multiSZ = % x, want % x", got, want)
	}
}
