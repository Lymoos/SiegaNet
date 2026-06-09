package clientnet

import (
	"net/netip"
	"testing"
)

func specFor(t *testing.T) []fwFilter {
	t.Helper()
	return killSwitchSpec(netip.MustParseAddr("203.0.113.7"), 0xdeadbeef)
}

func hasCond(f fwFilter, field fwField) bool {
	for _, c := range f.Conditions {
		if c.Field == field {
			return true
		}
	}
	return false
}

// TestDefaultDenyOnEveryLayer: every layer must carry a block-all.
func TestDefaultDenyOnEveryLayer(t *testing.T) {
	fs := specFor(t)
	for _, l := range []fwLayer{layerConnectV4, layerRecvV4, layerConnectV6, layerRecvV6} {
		found := false
		for _, f := range fs {
			if f.Layer == l && f.Action == actionBlock && len(f.Conditions) == 0 {
				found = true
			}
		}
		if !found {
			t.Errorf("no unconditional block-all on layer %v", l)
		}
	}
}

// TestPermitsOutrankBlocks: the lowest permit weight must exceed the highest
// block weight, or the permits would never take effect.
func TestPermitsOutrankBlocks(t *testing.T) {
	fs := specFor(t)
	maxBlock, minPermit := uint8(0), uint8(255)
	for _, f := range fs {
		switch f.Action {
		case actionBlock:
			if f.Weight > maxBlock {
				maxBlock = f.Weight
			}
		case actionPermit:
			if f.Weight < minPermit {
				minPermit = f.Weight
			}
		}
	}
	if minPermit <= maxBlock {
		t.Fatalf("permit weight %d does not outrank block weight %d", minPermit, maxBlock)
	}
}

// TestIPv6FullyBlocked: IPv6 layers may carry ONLY loopback permits; any other
// IPv6 permit would be a dual-stack leak past the IPv4 tunnel.
func TestIPv6FullyBlocked(t *testing.T) {
	fs := specFor(t)
	for _, f := range fs {
		if f.Layer.isV6() && f.Action == actionPermit {
			if !hasCond(f, fieldLoopback) {
				t.Errorf("non-loopback IPv6 permit %q would leak past the IPv4 tunnel", f.Name)
			}
		}
	}
	// And there must be a block on both v6 layers.
	for _, l := range []fwLayer{layerConnectV6, layerRecvV6} {
		blocked := false
		for _, f := range fs {
			if f.Layer == l && f.Action == actionBlock {
				blocked = true
			}
		}
		if !blocked {
			t.Errorf("IPv6 layer %v not blocked", l)
		}
	}
}

// TestDHCPPermitIsNarrow: the DHCP permit must be UDP with ports 68->67 only.
func TestDHCPPermitIsNarrow(t *testing.T) {
	fs := specFor(t)
	var dhcp *fwFilter
	for i := range fs {
		if fs[i].Name == "permit-dhcp" {
			dhcp = &fs[i]
		}
	}
	if dhcp == nil {
		t.Fatal("no DHCP permit")
	}
	if dhcp.Layer != layerConnectV4 {
		t.Errorf("DHCP permit on wrong layer %v", dhcp.Layer)
	}
	var proto uint8
	var lport, rport uint16
	for _, c := range dhcp.Conditions {
		switch c.Field {
		case fieldProtocol:
			proto = c.Proto
		case fieldLocalPort:
			lport = c.Port
		case fieldRemotePort:
			rport = c.Port
		default:
			t.Errorf("unexpected DHCP condition field %v (would widen the permit)", c.Field)
		}
	}
	if proto != protoUDP || lport != 68 || rport != 67 {
		t.Errorf("DHCP permit not narrow: proto=%d local=%d remote=%d (want udp 68->67)", proto, lport, rport)
	}
}

// TestEndpointAndTunnelPermits: the endpoint /32 and tunnel-LUID permits exist.
func TestEndpointAndTunnelPermits(t *testing.T) {
	fs := killSwitchSpec(netip.MustParseAddr("198.51.100.4"), 0x1234)
	var ep, tun bool
	for _, f := range fs {
		if f.Name == "permit-endpoint" {
			ep = true
			for _, c := range f.Conditions {
				if c.Field == fieldRemoteAddr && c.Addr != netip.MustParseAddr("198.51.100.4") {
					t.Errorf("endpoint permit has wrong addr %v", c.Addr)
				}
			}
		}
		if f.Name == "permit-tunnel" {
			tun = true
			if !hasCond(f, fieldLocalInterface) {
				t.Error("tunnel permit missing local-interface condition")
			}
		}
	}
	if !ep || !tun {
		t.Fatalf("missing permits: endpoint=%v tunnel=%v", ep, tun)
	}
}

// TestEndpointPermitSwappable: the isolated endpoint permit (for atomic failover)
// matches the one in the full set.
// TestConditionFWPTypes pins the FWP_DATA_TYPE per condition field. Getting any
// of these wrong makes the live WFP engine reject the filter ("wrong type") —
// exactly the first failure seen on real Windows (endpoint address was sent as
// FWP_V4_ADDR_MASK instead of FWP_UINT32).
func TestConditionFWPTypes(t *testing.T) {
	cases := []struct {
		field fwField
		dtype uint32
		byPtr bool
	}{
		{fieldRemoteAddr, fwpUint32, false}, // single IPv4 addr, INLINE host-order
		{fieldLocalInterface, fwpUint64, true},
		{fieldProtocol, fwpUint8, false},
		{fieldLocalPort, fwpUint16, false},
		{fieldRemotePort, fwpUint16, false},
		{fieldLoopback, fwpUint32, false},
	}
	for _, c := range cases {
		dt, bp := conditionFWPType(c.field)
		if dt != c.dtype || bp != c.byPtr {
			t.Errorf("conditionFWPType(%v) = (%d,%v), want (%d,%v)", c.field, dt, bp, c.dtype, c.byPtr)
		}
	}
}

func TestEndpointPermitSwappable(t *testing.T) {
	ep, ok := endpointPermit(netip.MustParseAddr("203.0.113.7"))
	if !ok {
		t.Fatal("endpointPermit returned !ok for a valid v4 addr")
	}
	if ep.Layer != layerConnectV4 || ep.Action != actionPermit {
		t.Errorf("endpoint permit wrong layer/action: %+v", ep)
	}
	if _, ok := endpointPermit(netip.Addr{}); ok {
		t.Error("endpointPermit accepted an invalid address")
	}
}
