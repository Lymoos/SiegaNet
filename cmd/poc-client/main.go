// Command sieganet-client is the SiegaNet client.
//
// Phase 0: dials the server over bare QUIC (skipping certificate verification,
// only in phase0_insecure builds), brings up a local TUN device and pumps IP
// packets over the connection's DATAGRAM channel, clamping the inner MTU to the
// runtime-probed max datagram size.
package main

import (
	"context"
	"flag"
	"log"
	"net"
	"os/signal"
	"syscall"

	"github.com/lymoos/sieganet/internal/transport"
	"github.com/lymoos/sieganet/internal/tundev"
	"github.com/lymoos/sieganet/internal/tunnel"
)

func main() {
	server := flag.String("server", "127.0.0.1:4443", "server address host:port")
	tunName := flag.String("tun", "siega0", "TUN interface name")
	tunIP := flag.String("tun-ip", "10.7.0.2/24", "inner IP/CIDR for the TUN interface")
	route := flag.String("route", "", "extra route to send over the tunnel (the /24 from -tun-ip is on-link already)")
	mtu := flag.Int("mtu", 1280, "upper bound for the inner MTU (runtime may lower it)")
	rxWorkers := flag.Int("rx-workers", 1, "datagram receive workers (>1 drains quic-go's rcv queue faster on fast links, may reorder)")
	flag.Parse()

	log.SetPrefix("[poc-client] ")
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
	localIP := mustIP(*tunIP)
	if *route != "" {
		if err := tundev.AddRoute(name, *route); err != nil {
			log.Fatalf("add route %s: %v", *route, err)
		}
	}
	log.Printf("tun %s up with %s, route %q", name, *tunIP, *route)

	tlsConf, err := transport.InsecureClientTLS(transport.ALPNPoC)
	if err != nil {
		log.Fatalf("tls: %v", err)
	}
	ep, err := transport.NewClientEndpoint()
	if err != nil {
		log.Fatalf("endpoint: %v", err)
	}
	defer ep.Close()
	rcv, snd := ep.BufferSizes()
	log.Printf("udp socket buffers: rcvbuf=%d sndbuf=%d", rcv, snd)

	conn, err := ep.Dial(ctx, *server, tlsConf)
	if err != nil {
		log.Fatalf("dial %s: %v", *server, err)
	}
	log.Printf("connected to %s (quic, datagrams enabled)", conn.RemoteAddr())

	maxDg, err := transport.MaxDatagramSize(conn)
	if err != nil {
		log.Fatalf("probe max datagram: %v", err)
	}
	inner := tunnel.InnerMTU(maxDg, 0, *mtu)
	if err := tundev.SetMTU(name, inner); err != nil {
		log.Printf("set mtu: %v", err)
	}
	if err := tundev.ClampMSS(name, inner); err != nil {
		log.Printf("clamp mss: %v", err)
	}
	defer tundev.UnclampMSS(name, inner)
	log.Printf("mtu gate: max_datagram=%d inner_mtu=%d (mss<=%d)", maxDg, inner, inner-40)

	stats := &tunnel.Stats{}
	cfg := tunnel.Config{
		MaxDatagram: maxDg,
		PadMax:      0,
		LocalTunIP:  localIP,
		Stats:       stats,
		RxWorkers:   *rxWorkers,
		OnMTULowered: func(m int) {
			if err := tundev.SetMTU(name, m); err != nil {
				log.Printf("re-clamp mtu: %v", err)
			}
		},
	}
	err = tunnel.Run(ctx, dev, conn, cfg)
	log.Printf("tunnel down: %v", err)
	log.Print(stats.String())
	if rxD, txD, e := tundev.IfaceDrops(name); e == nil {
		log.Printf("iface %s kernel drops: rx_dropped=%d tx_dropped=%d", name, rxD, txD)
	}
}

func mustIP(cidr string) net.IP {
	ip, _, err := net.ParseCIDR(cidr)
	if err != nil {
		log.Fatalf("parse tun-ip %q: %v", cidr, err)
	}
	return ip
}
