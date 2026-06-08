// Package clientnet provides the OS-specific client network configuration
// (clientcore.Configurator). This file holds the OS-neutral route PLAN, so the
// route set and — critically — the ordering can be unit-tested anywhere; the
// per-OS files execute the plan against the real network stack.
package clientnet

import "net/netip"

// RouteOp is one route to install, in execution order.
type RouteOp struct {
	Dest      netip.Prefix // route destination
	ViaTunnel bool         // true: send via the tunnel interface; false: keep it
	//                        off the tunnel, via the physical next-hop
	Why string // human-readable purpose (for logs/tests)
}

// fullTunnelHalves splits the default route into two more-specific halves so the
// original default (0.0.0.0/0) is overridden without being deleted — the
// WireGuard scheme.
var fullTunnelHalves = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/1"),
	netip.MustParsePrefix("128.0.0.0/1"),
}

// PlanRoutes returns the ordered list of route operations for a client.
//
// ORDER IS A CORRECTNESS REQUIREMENT, not an optimisation: the server-endpoint
// host route (endpoint/32, kept OFF the tunnel via the physical gateway) is
// always emitted FIRST, before the 0.0.0.0/1 + 128.0.0.0/1 full-tunnel routes
// that redirect the default. If the order were reversed, the very first packet
// to the server (and every reconnect handshake) could be captured by the tunnel
// that is not yet — or, on teardown, no longer — up, looping the QUIC connection
// into itself. The caller MUST install these in the returned order.
func PlanRoutes(endpoint netip.Addr, fullTunnel bool) []RouteOp {
	ops := make([]RouteOp, 0, 3)
	if endpoint.IsValid() {
		ops = append(ops, RouteOp{
			Dest:      netip.PrefixFrom(endpoint, endpoint.BitLen()), // /32 (or /128)
			ViaTunnel: false,
			Why:       "server endpoint via physical gateway (anti-loop; must precede full-tunnel)",
		})
	}
	if fullTunnel {
		for _, h := range fullTunnelHalves {
			ops = append(ops, RouteOp{Dest: h, ViaTunnel: true, Why: "full-tunnel default override"})
		}
	}
	return ops
}
