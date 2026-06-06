// Command siega-impair is a TEST-ONLY UDP impairment relay. It is NOT part of
// the VPN: it sits between the client and the server on the UDP path and adds a
// fixed one-way delay and random per-packet loss in both directions, emulating
// a mobile-like link when the kernel's tc-netem qdisc is unavailable.
//
// Client  --UDP-->  siega-impair  --UDP-->  server   (and the reverse).
// The client dials the relay's address instead of the server's.
//
//	siega-impair -listen 10.0.0.1:4444 -target 127.0.0.1:4443 -delay 60ms -loss 0.02
package main

import (
	"flag"
	"log"
	"math/rand/v2"
	"net"
	"sync/atomic"
	"time"
)

func main() {
	listen := flag.String("listen", ":4444", "UDP address to listen on (client dials this)")
	target := flag.String("target", "127.0.0.1:4443", "real server UDP address to relay to")
	delay := flag.Duration("delay", 60*time.Millisecond, "one-way delay added in each direction")
	loss := flag.Float64("loss", 0.02, "per-packet drop probability in each direction (0..1)")
	flag.Parse()

	log.SetPrefix("[siega-impair] ")

	laddr, err := net.ResolveUDPAddr("udp", *listen)
	if err != nil {
		log.Fatalf("resolve listen: %v", err)
	}
	taddr, err := net.ResolveUDPAddr("udp", *target)
	if err != nil {
		log.Fatalf("resolve target: %v", err)
	}
	front, err := net.ListenUDP("udp", laddr)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	// Socket facing the server; its source port is what the server treats as the
	// "client". One client at a time is enough for the Phase 0 test.
	up, err := net.DialUDP("udp", nil, taddr)
	if err != nil {
		log.Fatalf("dial target: %v", err)
	}
	log.Printf("relaying %s -> %s with delay=%s loss=%.3f (each direction)", *listen, *target, *delay, *loss)

	var clientAddr atomic.Pointer[net.UDPAddr]
	var dropped, passed atomic.Uint64

	send := func(deliver func(b []byte)) func(b []byte) {
		return func(b []byte) {
			if rand.Float64() < *loss {
				dropped.Add(1)
				return
			}
			passed.Add(1)
			cp := make([]byte, len(b))
			copy(cp, b)
			// Constant delay preserves ordering, so a per-packet timer is fine.
			time.AfterFunc(*delay, func() { deliver(cp) })
		}
	}

	// client -> server
	toServer := send(func(b []byte) { _, _ = up.Write(b) })
	// server -> client
	toClient := send(func(b []byte) {
		if ca := clientAddr.Load(); ca != nil {
			_, _ = front.WriteToUDP(b, ca)
		}
	})

	// server -> client reader
	go func() {
		buf := make([]byte, 65535)
		for {
			n, err := up.Read(buf)
			if err != nil {
				log.Printf("upstream read: %v", err)
				return
			}
			toClient(buf[:n])
		}
	}()

	// periodic impairment summary
	go func() {
		for range time.Tick(5 * time.Second) {
			log.Printf("relayed=%d dropped=%d", passed.Load(), dropped.Load())
		}
	}()

	// client -> server reader
	buf := make([]byte, 65535)
	for {
		n, addr, err := front.ReadFromUDP(buf)
		if err != nil {
			log.Printf("front read: %v", err)
			return
		}
		clientAddr.Store(addr)
		toServer(buf[:n])
	}
}
