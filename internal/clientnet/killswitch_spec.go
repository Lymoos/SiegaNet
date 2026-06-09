package clientnet

import "net/netip"

// This file is the OS-neutral specification of the WFP kill-switch: the abstract
// set of permit/block filters and their conditions. Keeping the POLICY here makes
// every invariant the design depends on unit-testable anywhere, independent of
// the Windows WFP plumbing that realises it:
//
//   - default-deny: everything is blocked unless explicitly permitted;
//   - permits outrank blocks (weight), so the permits actually take effect;
//   - the ONLY things reachable off the tunnel are loopback, narrow DHCP
//     (UDP 68->67), and the server endpoint /32 — nothing else can leak;
//   - IPv6 is fully blocked (no IPv6 permit except loopback), since the tunnel
//     is IPv4: a dual-stack host must not leak past the IPv4 tunnel over IPv6.
//
// The Windows file translates this spec into FWPM filters; it adds no policy of
// its own, so the security-relevant decisions are all covered by the tests here.

// fwAction is permit or block.
type fwAction int

const (
	actionBlock fwAction = iota
	actionPermit
)

// fwLayer selects the WFP ALE layer (and thus IP version + direction).
type fwLayer int

const (
	layerConnectV4 fwLayer = iota // outbound IPv4
	layerRecvV4                   // inbound IPv4
	layerConnectV6                // outbound IPv6
	layerRecvV6                   // inbound IPv6
)

func (l fwLayer) isV6() bool { return l == layerConnectV6 || l == layerRecvV6 }

// fwField identifies a match condition's field.
type fwField int

const (
	fieldLoopback       fwField = iota // FWPM_CONDITION_FLAGS & IS_LOOPBACK
	fieldLocalInterface                // FWPM_CONDITION_IP_LOCAL_INTERFACE (LUID)
	fieldRemoteAddr                    // FWPM_CONDITION_IP_REMOTE_ADDRESS
	fieldProtocol                      // FWPM_CONDITION_IP_PROTOCOL
	fieldLocalPort                     // FWPM_CONDITION_IP_LOCAL_PORT
	fieldRemotePort                    // FWPM_CONDITION_IP_REMOTE_PORT
)

const protoUDP uint8 = 17

// fwCondition is one match condition.
type fwCondition struct {
	Field fwField
	LUID  uint64     // fieldLocalInterface
	Addr  netip.Addr // fieldRemoteAddr
	Proto uint8      // fieldProtocol
	Port  uint16     // fieldLocal/RemotePort
}

// Filter weights: permits must outrank blocks so they win at the same layer.
const (
	weightBlock  uint8 = 1
	weightPermit uint8 = 12
)

// fwFilter is one filter in the kill-switch set.
type fwFilter struct {
	Name       string
	Layer      fwLayer
	Action     fwAction
	Weight     uint8
	Conditions []fwCondition
}

// killSwitchSpec returns the full filter set. endpoint/endpointPort identify the
// server; tunnelLUID is the wintun adapter. Installed in one WFP transaction
// before the first dial and kept up for the whole session (fail-closed).
func killSwitchSpec(endpoint netip.Addr, tunnelLUID uint64) []fwFilter {
	var fs []fwFilter

	// Default-deny on every layer (both IP versions, both directions). IPv6 gets
	// only this block (no IPv6 permit below except loopback) => IPv6 fully dead.
	for _, l := range []fwLayer{layerConnectV4, layerRecvV4, layerConnectV6, layerRecvV6} {
		fs = append(fs, fwFilter{Name: "block-all", Layer: l, Action: actionBlock, Weight: weightBlock})
	}

	// Permit loopback (all layers) so local IPC keeps working.
	for _, l := range []fwLayer{layerConnectV4, layerRecvV4, layerConnectV6, layerRecvV6} {
		fs = append(fs, fwFilter{
			Name: "permit-loopback", Layer: l, Action: actionPermit, Weight: weightPermit,
			Conditions: []fwCondition{{Field: fieldLoopback}},
		})
	}

	// Permit all traffic on the tunnel interface (IPv4 only — the inner net).
	// Skipped when tunnelLUID == 0: at kill-switch-engage time the adapter does
	// not exist yet, so these are added later by tunnelPermits() once it does.
	if tunnelLUID != 0 {
		fs = append(fs, tunnelPermits(tunnelLUID)...)
	}

	// Permit narrow DHCPv4 (client 68 -> server 67, UDP) so the physical link can
	// get/refresh a lease. Scoped to UDP + those exact ports: not a leak channel.
	fs = append(fs, fwFilter{
		Name: "permit-dhcp", Layer: layerConnectV4, Action: actionPermit, Weight: weightPermit,
		Conditions: []fwCondition{
			{Field: fieldProtocol, Proto: protoUDP},
			{Field: fieldLocalPort, Port: 68},
			{Field: fieldRemotePort, Port: 67},
		},
	})

	// Permit the server endpoint /32 off the tunnel (the QUIC handshake/keepalive
	// reaches the VPS). Updated atomically on failover (Phase 4).
	if endpoint.IsValid() && endpoint.Is4() {
		fs = append(fs, fwFilter{
			Name: "permit-endpoint", Layer: layerConnectV4, Action: actionPermit, Weight: weightPermit,
			Conditions: []fwCondition{{Field: fieldRemoteAddr, Addr: endpoint}},
		})
	}

	return fs
}

// tunnelPermits returns the tunnel-interface permits, installed once the wintun
// adapter (and thus its LUID) exists.
func tunnelPermits(luid uint64) []fwFilter {
	var fs []fwFilter
	for _, l := range []fwLayer{layerConnectV4, layerRecvV4} {
		fs = append(fs, fwFilter{
			Name: "permit-tunnel", Layer: l, Action: actionPermit, Weight: weightPermit,
			Conditions: []fwCondition{{Field: fieldLocalInterface, LUID: luid}},
		})
	}
	return fs
}

// endpointPermit isolates just the endpoint filter, so failover can swap it in a
// single WFP transaction without disturbing the rest of the set.
func endpointPermit(endpoint netip.Addr) (fwFilter, bool) {
	if !endpoint.IsValid() || !endpoint.Is4() {
		return fwFilter{}, false
	}
	return fwFilter{
		Name: "permit-endpoint", Layer: layerConnectV4, Action: actionPermit, Weight: weightPermit,
		Conditions: []fwCondition{{Field: fieldRemoteAddr, Addr: endpoint}},
	}, true
}
