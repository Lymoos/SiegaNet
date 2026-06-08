package clientnet

import (
	"net/netip"
	"testing"
)

func TestPlanRoutesFullTunnelSetAndOrder(t *testing.T) {
	ep := netip.MustParseAddr("203.0.113.5")
	ops := PlanRoutes(ep, true)
	if len(ops) != 3 {
		t.Fatalf("got %d ops, want 3: %+v", len(ops), ops)
	}
	// [0] endpoint /32, off-tunnel.
	if ops[0].Dest != netip.MustParsePrefix("203.0.113.5/32") || ops[0].ViaTunnel {
		t.Errorf("op0 = %+v, want 203.0.113.5/32 off-tunnel", ops[0])
	}
	// [1],[2] the two full-tunnel halves, on-tunnel.
	if ops[1].Dest != netip.MustParsePrefix("0.0.0.0/1") || !ops[1].ViaTunnel {
		t.Errorf("op1 = %+v, want 0.0.0.0/1 via tunnel", ops[1])
	}
	if ops[2].Dest != netip.MustParsePrefix("128.0.0.0/1") || !ops[2].ViaTunnel {
		t.Errorf("op2 = %+v, want 128.0.0.0/1 via tunnel", ops[2])
	}
}

// TestEndpointPrecedesFullTunnel is the anti-loop invariant: the endpoint
// exclusion route MUST be installed before any route that sends traffic into the
// tunnel, or the first packet to the server could be captured by the tunnel.
func TestEndpointPrecedesFullTunnel(t *testing.T) {
	ops := PlanRoutes(netip.MustParseAddr("198.51.100.9"), true)
	endpointIdx := -1
	firstTunnelIdx := len(ops)
	for i, op := range ops {
		if !op.ViaTunnel && op.Dest.Bits() == op.Dest.Addr().BitLen() {
			endpointIdx = i
		}
		if op.ViaTunnel && i < firstTunnelIdx {
			firstTunnelIdx = i
		}
	}
	if endpointIdx == -1 {
		t.Fatal("no endpoint exclusion route planned")
	}
	if endpointIdx >= firstTunnelIdx {
		t.Fatalf("endpoint route at %d must precede the first tunnel route at %d", endpointIdx, firstTunnelIdx)
	}
}

func TestPlanRoutesNoFullTunnel(t *testing.T) {
	ops := PlanRoutes(netip.MustParseAddr("203.0.113.5"), false)
	if len(ops) != 1 || ops[0].ViaTunnel {
		t.Fatalf("want only the off-tunnel exclusion, got %+v", ops)
	}
}

func TestPlanRoutesNoEndpoint(t *testing.T) {
	// An invalid endpoint yields just the full-tunnel routes (caller resolves it
	// before this in practice; defensive).
	ops := PlanRoutes(netip.Addr{}, true)
	for _, op := range ops {
		if !op.ViaTunnel {
			t.Errorf("unexpected off-tunnel op without an endpoint: %+v", op)
		}
	}
}
