// Command siega-impair is a TEST-ONLY UDP impairment relay. It is NOT part of
// the VPN: it sits between the client and the server on the UDP path and adds a
// fixed one-way delay and random per-packet loss in both directions, emulating
// a mobile-like link when the kernel's tc-netem qdisc is unavailable.
//
// Client  --UDP-->  siega-impair  --UDP-->  server   (and the reverse).
// The client dials the relay's address instead of the server's.
//
//	siega-impair -listen 10.0.0.1:4444 -target 127.0.0.1:4443 -delay 60ms -loss 0.02
//
// Each direction has a single delivery goroutine draining an ordered queue, so
// packets are delivered in arrival order with a constant delay and writes never
// race — otherwise a per-packet timer would reorder packets under load and the
// inner TCP would misread that as loss.
package main

import (
	"flag"
	"log"
	"math/rand/v2"
	"net"
	"sync/atomic"
	"time"
)

const queueDepth = 8192

type item struct {
	data []byte
	due  time.Time
}

// newImpairDir returns a function that applies loss then enqueues a packet for
// in-order delivery after delay. A single consumer goroutine serializes writes.
func newImpairDir(delay time.Duration, loss float64, write func([]byte), dropped, passed *atomic.Uint64) func([]byte) {
	ch := make(chan item, queueDepth)
	go func() {
		for it := range ch {
			if d := time.Until(it.due); d > 0 {
				time.Sleep(d)
			}
			write(it.data)
		}
	}()
	return func(b []byte) {
		if rand.Float64() < loss {
			dropped.Add(1)
			return
		}
		cp := make([]byte, len(b))
		copy(cp, b)
		select {
		case ch <- item{cp, time.Now().Add(delay)}:
			passed.Add(1)
		default:
			dropped.Add(1) // queue overload; count as a drop
		}
	}
}

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

	toServer := newImpairDir(*delay, *loss, func(b []byte) { _, _ = up.Write(b) }, &dropped, &passed)
	toClient := newImpairDir(*delay, *loss, func(b []byte) {
		if ca := clientAddr.Load(); ca != nil {
			_, _ = front.WriteToUDP(b, ca)
		}
	}, &dropped, &passed)

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
