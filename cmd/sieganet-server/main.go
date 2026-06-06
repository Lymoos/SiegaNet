// Command sieganet-server is the SiegaNet server.
//
// Phase 0: a bare QUIC<->TUN bridge with no masking, no authentication and a
// self-signed certificate. It accepts one tunnel connection at a time and pumps
// IP packets between the connection's DATAGRAM channel and a local TUN device.
// Masking, the decoy site, WebTransport and HMAC peer auth arrive in Phase 1.
package main

import (
	"context"
	"flag"
	"log"
	"os/signal"
	"syscall"

	"github.com/lymoos/sieganet/internal/protocol"
	"github.com/lymoos/sieganet/internal/transport"
	"github.com/lymoos/sieganet/internal/tundev"
	"github.com/lymoos/sieganet/internal/tunnel"
)

func main() {
	listen := flag.String("listen", ":4443", "UDP address to listen on")
	tunName := flag.String("tun", "siega0", "TUN interface name")
	tunIP := flag.String("tun-ip", "10.7.0.1/24", "inner IP/CIDR for the TUN interface")
	mtu := flag.Int("mtu", 1280, "TUN MTU")
	flag.Parse()

	log.SetPrefix("[sieganet-server] ")
	log.Printf("Phase 0 PoC — self-signed TLS, no auth, no masking")

	dev, err := tundev.Open(*tunName, *mtu)
	if err != nil {
		log.Fatalf("open tun: %v", err)
	}
	defer dev.Close()
	name, _ := tundev.ActualName(dev)
	if err := tundev.ConfigureInterface(name, *tunIP); err != nil {
		log.Fatalf("configure %s: %v", name, err)
	}
	log.Printf("tun %s up with %s (mtu %d)", name, *tunIP, *mtu)

	tlsConf, err := transport.SelfSignedTLS(transport.ALPNPoC)
	if err != nil {
		log.Fatalf("tls: %v", err)
	}
	ln, err := transport.Listen(*listen, tlsConf)
	if err != nil {
		log.Fatalf("listen %s: %v", *listen, err)
	}
	defer ln.Close()
	log.Printf("listening on %s (quic, datagrams enabled)", *listen)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	for {
		conn, err := ln.Accept(ctx)
		if err != nil {
			if ctx.Err() != nil {
				log.Printf("shutting down")
				return
			}
			log.Printf("accept: %v", err)
			continue
		}
		log.Printf("tunnel up: %s", conn.RemoteAddr())
		// Phase 0 serves a single tunnel at a time.
		err = tunnel.Run(ctx, dev, conn, protocol.PadRange{})
		log.Printf("tunnel down: %s (%v)", conn.RemoteAddr(), err)
		if ctx.Err() != nil {
			return
		}
	}
}
