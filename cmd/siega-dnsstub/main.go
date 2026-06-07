// Command siega-dnsstub is a TEST-ONLY minimal UDP DNS server that answers every
// A query with a fixed address and logs the queried name. It stands in for the
// tunnel-side resolver in the end-to-end DNS-leak test: a successful answer from
// here proves the client's DNS query travelled through the tunnel.
package main

import (
	"encoding/binary"
	"flag"
	"log"
	"net"
)

func main() {
	listen := flag.String("listen", ":53", "UDP address to listen on")
	answer := flag.String("answer", "203.0.113.55", "A record to return for every query")
	flag.Parse()

	ip := net.ParseIP(*answer).To4()
	if ip == nil {
		log.Fatalf("bad answer IP %q", *answer)
	}
	addr, err := net.ResolveUDPAddr("udp", *listen)
	if err != nil {
		log.Fatal(err)
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("dns-stub listening on %s, answering A -> %s", *listen, ip)

	buf := make([]byte, 1500)
	for {
		n, from, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Printf("read: %v", err)
			continue
		}
		resp := buildResponse(buf[:n], ip)
		if resp == nil {
			continue
		}
		log.Printf("query from %s: %s", from, qname(buf[:n]))
		_, _ = conn.WriteToUDP(resp, from)
	}
}

// buildResponse turns a query into a response with a single A answer.
func buildResponse(q []byte, ip net.IP) []byte {
	if len(q) < 12 {
		return nil
	}
	// Find the end of the question (QNAME terminator + QTYPE + QCLASS).
	i := 12
	for i < len(q) && q[i] != 0 {
		i += int(q[i]) + 1
	}
	qend := i + 1 + 4 // null label + qtype(2) + qclass(2)
	if qend > len(q) {
		return nil
	}
	resp := make([]byte, 0, qend+16)
	resp = append(resp, q[:qend]...)
	binary.BigEndian.PutUint16(resp[2:], 0x8180) // response, recursion available
	binary.BigEndian.PutUint16(resp[6:], 1)      // ANCOUNT = 1
	binary.BigEndian.PutUint16(resp[8:], 0)      // NSCOUNT = 0
	binary.BigEndian.PutUint16(resp[10:], 0)     // ARCOUNT = 0 (drop any EDNS OPT)
	// Answer: name pointer to offset 12, A/IN, TTL 60, RDLENGTH 4, RDATA.
	ans := []byte{0xC0, 0x0C, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x3C, 0x00, 0x04}
	ans = append(ans, ip...)
	return append(resp, ans...)
}

func qname(q []byte) string {
	if len(q) < 13 {
		return "?"
	}
	var out []byte
	i := 12
	for i < len(q) && q[i] != 0 {
		l := int(q[i])
		if i+1+l > len(q) {
			break
		}
		if len(out) > 0 {
			out = append(out, '.')
		}
		out = append(out, q[i+1:i+1+l]...)
		i += l + 1
	}
	return string(out)
}
