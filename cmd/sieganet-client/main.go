// Command sieganet-client is the SiegaNet client.
//
// Phase 0: dials the server over bare QUIC (skipping certificate verification),
// brings up a local TUN device and pumps IP packets over the connection's
// DATAGRAM channel. Real certificate verification, WebTransport, peer auth,
// full-tunnel routing and the kill-switch arrive in later phases.
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
	server := flag.String("server", "127.0.0.1:4443", "server address host:port")
	tunName := flag.String("tun", "siega0", "TUN interface name")
	tunIP := flag.String("tun-ip", "10.7.0.2/24", "inner IP/CIDR for the TUN interface")
	route := flag.String("route", "", "extra route to send over the tunnel (the /24 from -tun-ip is on-link already)")
	mtu := flag.Int("mtu", 1280, "TUN MTU")
	flag.Parse()

	log.SetPrefix("[sieganet-client] ")
	log.Printf("Phase 0 PoC — InsecureSkipVerify, no masking")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	dev, err := tundev.Open(*tunName, *mtu)
	if err != nil {
		log.Fatalf("open tun: %v", err)
	}
	defer dev.Close()
	name, _ := tundev.ActualName(dev)
	if err := tundev.ConfigureInterface(name, *tunIP); err != nil {
		log.Fatalf("configure %s: %v", name, err)
	}
	if *route != "" {
		if err := tundev.AddRoute(name, *route); err != nil {
			log.Fatalf("add route %s: %v", *route, err)
		}
	}
	log.Printf("tun %s up with %s (mtu %d), route %s", name, *tunIP, *mtu, *route)

	tlsConf := transport.InsecureClientTLS(transport.ALPNPoC)
	conn, err := transport.Dial(ctx, *server, tlsConf)
	if err != nil {
		log.Fatalf("dial %s: %v", *server, err)
	}
	log.Printf("connected to %s (quic, datagrams enabled)", conn.RemoteAddr())

	err = tunnel.Run(ctx, dev, conn, protocol.PadRange{})
	log.Printf("tunnel down: %v", err)
}
