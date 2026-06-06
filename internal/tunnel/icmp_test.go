package tunnel

import (
	"encoding/binary"
	"net"
	"testing"
)

// makeIPv4 builds a minimal IPv4 header for testing.
func makeIPv4(src, dst net.IP, df bool, payload []byte) []byte {
	pkt := make([]byte, 20+len(payload))
	pkt[0] = 0x45
	total := uint16(20 + len(payload))
	binary.BigEndian.PutUint16(pkt[2:], total)
	if df {
		binary.BigEndian.PutUint16(pkt[6:], ipv4FlagDF)
	}
	pkt[8] = 64
	pkt[9] = 6 // TCP
	copy(pkt[12:], src.To4())
	copy(pkt[16:], dst.To4())
	binary.BigEndian.PutUint16(pkt[10:], checksum(pkt[:20]))
	copy(pkt[20:], payload)
	return pkt
}

func TestDFDetection(t *testing.T) {
	src := net.IPv4(10, 7, 0, 2)
	dst := net.IPv4(1, 1, 1, 1)
	if ipv4HasDF(makeIPv4(src, dst, false, nil)) {
		t.Error("DF reported set when clear")
	}
	if !ipv4HasDF(makeIPv4(src, dst, true, nil)) {
		t.Error("DF reported clear when set")
	}
}

func TestChecksumKnownAnswer(t *testing.T) {
	// A correctly-checksummed header must checksum (including the field) to 0.
	pkt := makeIPv4(net.IPv4(192, 168, 0, 1), net.IPv4(192, 168, 0, 2), false, nil)
	if got := checksum(pkt[:20]); got != 0 {
		t.Errorf("checksum over valid header = %#x, want 0", got)
	}
}

func TestBuildICMPFragNeeded(t *testing.T) {
	src := net.IPv4(10, 7, 0, 9)      // the app that sent the oversized packet
	dst := net.IPv4(93, 184, 216, 34) // its destination
	router := net.IPv4(10, 7, 0, 2)   // tunnel pretends to be this hop
	orig := makeIPv4(src, dst, true, []byte{0xde, 0xad, 0xbe, 0xef, 0x01, 0x02, 0x03, 0x04, 0x05})

	icmp := buildICMPv4FragNeeded(orig, router, 1200)
	if icmp == nil {
		t.Fatal("nil ICMP")
	}
	// Outer IP header checks.
	if icmp[0]>>4 != 4 {
		t.Fatal("not IPv4")
	}
	if icmp[9] != protoICMP {
		t.Errorf("protocol = %d, want %d (ICMP)", icmp[9], protoICMP)
	}
	if !net.IP(icmp[12:16]).Equal(router.To4()) {
		t.Errorf("ICMP source = %v, want router %v", net.IP(icmp[12:16]), router)
	}
	if !net.IP(icmp[16:20]).Equal(src.To4()) {
		t.Errorf("ICMP dest = %v, want original source %v", net.IP(icmp[16:20]), src)
	}
	if got := checksum(icmp[:20]); got != 0 {
		t.Errorf("outer IP checksum invalid: %#x", got)
	}
	// ICMP message checks.
	msg := icmp[20:]
	if msg[0] != 3 || msg[1] != 4 {
		t.Errorf("ICMP type/code = %d/%d, want 3/4", msg[0], msg[1])
	}
	if mtu := binary.BigEndian.Uint16(msg[6:]); mtu != 1200 {
		t.Errorf("next-hop MTU = %d, want 1200", mtu)
	}
	if got := checksum(msg); got != 0 {
		t.Errorf("ICMP checksum invalid: %#x", got)
	}
	// The embedded copy must start with the original IP header.
	embedded := msg[8:]
	if embedded[0] != orig[0] || !net.IP(embedded[12:16]).Equal(src.To4()) {
		t.Error("embedded original header mismatch")
	}
}

func TestBuildICMPRejectsNonIPv4(t *testing.T) {
	if buildICMPv4FragNeeded([]byte{0x60, 0, 0, 0}, net.IPv4(10, 0, 0, 1), 1200) != nil {
		t.Error("expected nil for IPv6 packet")
	}
	if buildICMPv4FragNeeded(nil, net.IPv4(10, 0, 0, 1), 1200) != nil {
		t.Error("expected nil for empty packet")
	}
}
