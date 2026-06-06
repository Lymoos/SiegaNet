package tunnel

import (
	"encoding/binary"
	"net"
)

// IPv4 header field offsets / flags we need for the MTU gate.
const (
	ipv4VerIHL    = 0
	ipv4FlagsFrag = 6 // 2 bytes: 3 flag bits + 13 fragment-offset bits
	ipv4Proto     = 9
	ipv4Src       = 12
	ipv4Dst       = 16
	ipv4FlagDF    = 0x4000 // Don't-Fragment bit within the flags/frag field
	protoICMP     = 1
)

// isIPv4 reports whether pkt looks like an IPv4 packet.
func isIPv4(pkt []byte) bool {
	return len(pkt) >= 20 && pkt[ipv4VerIHL]>>4 == 4
}

// ipv4HeaderLen returns the IPv4 header length in bytes (IHL * 4).
func ipv4HeaderLen(pkt []byte) int {
	return int(pkt[ipv4VerIHL]&0x0f) * 4
}

// ipv4HasDF reports whether the Don't-Fragment bit is set.
func ipv4HasDF(pkt []byte) bool {
	if len(pkt) < ipv4FlagsFrag+2 {
		return false
	}
	return binary.BigEndian.Uint16(pkt[ipv4FlagsFrag:])&ipv4FlagDF != 0
}

// ipv4Source returns the source address of an IPv4 packet.
func ipv4Source(pkt []byte) net.IP {
	if len(pkt) < ipv4Src+4 {
		return nil
	}
	return net.IPv4(pkt[ipv4Src], pkt[ipv4Src+1], pkt[ipv4Src+2], pkt[ipv4Src+3])
}

// checksum computes the 16-bit one's-complement Internet checksum (RFC 1071).
func checksum(b []byte) uint16 {
	var sum uint32
	for i := 0; i+1 < len(b); i += 2 {
		sum += uint32(binary.BigEndian.Uint16(b[i:]))
	}
	if len(b)%2 == 1 {
		sum += uint32(b[len(b)-1]) << 8
	}
	for sum>>16 != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return ^uint16(sum)
}

// buildICMPv4FragNeeded crafts an ICMPv4 "Destination Unreachable: Fragmentation
// Needed and DF set" (type 3, code 4) error for orig, advertising nextHopMTU.
// routerIP is used as the source — i.e. the tunnel pretends to be the first hop
// that couldn't forward the oversized packet, so the local stack lowers its
// path MTU instead of silently losing the packet (PMTU blackhole).
//
// The returned packet is meant to be written back into the TUN so the local IP
// stack receives it. Returns nil if orig is not a usable IPv4 packet.
func buildICMPv4FragNeeded(orig []byte, routerIP net.IP, nextHopMTU int) []byte {
	if !isIPv4(orig) {
		return nil
	}
	r4 := routerIP.To4()
	if r4 == nil {
		return nil
	}
	origSrc := orig[ipv4Src : ipv4Src+4]

	// RFC 792/1812: include the original IP header plus the first 8 bytes of its
	// payload so the sender can match the error to a socket.
	embedLen := ipv4HeaderLen(orig) + 8
	if embedLen > len(orig) {
		embedLen = len(orig)
	}

	const ipHdr = 20
	const icmpHdr = 8 // type, code, checksum, unused(2), next-hop MTU(2)
	total := ipHdr + icmpHdr + embedLen
	pkt := make([]byte, total)

	// --- IPv4 header ---
	pkt[0] = 0x45 // version 4, IHL 5
	binary.BigEndian.PutUint16(pkt[2:], uint16(total))
	pkt[8] = 64        // TTL
	pkt[9] = protoICMP // protocol
	copy(pkt[ipv4Src:], r4)
	copy(pkt[ipv4Dst:], origSrc)
	binary.BigEndian.PutUint16(pkt[10:], checksum(pkt[:ipHdr]))

	// --- ICMP message ---
	icmp := pkt[ipHdr:]
	icmp[0] = 3 // Destination Unreachable
	icmp[1] = 4 // Fragmentation Needed and DF set
	// icmp[4:6] unused, left zero.
	binary.BigEndian.PutUint16(icmp[6:], uint16(nextHopMTU))
	copy(icmp[icmpHdr:], orig[:embedLen])
	binary.BigEndian.PutUint16(icmp[2:], checksum(icmp[:icmpHdr+embedLen]))

	return pkt
}
