// Command sieganet-server is the SiegaNet server.
//
// Phase 0: a bare QUIC<->TUN bridge with no masking, no authentication and a
// self-signed certificate (only available in builds tagged phase0_insecure).
// It accepts one tunnel connection at a time and pumps IP packets between the
// connection's DATAGRAM channel and a local TUN device, clamping the inner MTU
// to the runtime-probed max datagram size.
package main

import (
	"context"
	"flag"
	"log"
	"net"
	"os/signal"
	"syscall"

	"github.com/quic-go/quic-go"
	"golang.zx2c4.com/wireguard/tun"

	"github.com/lymoos/sieganet/internal/transport"
	"github.com/lymoos/sieganet/internal/tundev"
	"github.com/lymoos/sieganet/internal/tunnel"
)

func main() {
	listen := flag.String("listen", ":4443", "UDP address to listen on")
	tunName := flag.String("tun", "siega0", "TUN interface name")
	tunIP := flag.String("tun-ip", "10.7.0.1/24", "inner IP/CIDR for the TUN interface")
	mtu := flag.Int("mtu", 1280, "upper bound for the inner MTU (runtime may lower it)")
	rxWorkers := flag.Int("rx-workers", 1, "datagram receive workers (>1 drains quic-go's rcv queue faster on fast links, may reorder)")
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
	localIP := mustIP(*tunIP)
	log.Printf("tun %s up with %s", name, *tunIP)

	tlsConf, err := transport.SelfSignedTLS(transport.ALPNPoC)
	if err != nil {
		log.Fatalf("tls: %v", err)
	}
	ep, err := transport.NewServerEndpoint(*listen)
	if err != nil {
		log.Fatalf("endpoint: %v", err)
	}
	defer ep.Close()
	rcv, snd := ep.BufferSizes()
	log.Printf("udp socket buffers: rcvbuf=%d sndbuf=%d", rcv, snd)

	ln, err := ep.Listen(tlsConf)
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
		serve(ctx, dev, name, localIP, *mtu, *rxWorkers, conn)
		if ctx.Err() != nil {
			return
		}
	}
}

// serve clamps the MTU to the connection and pumps the data-plane for one peer.
func serve(ctx context.Context, dev tun.Device, name string, localIP net.IP, upperMTU, rxWorkers int, conn *quic.Conn) {
	maxDg, err := transport.MaxDatagramSize(conn)
	if err != nil {
		log.Printf("probe max datagram: %v", err)
		conn.CloseWithError(0, "")
		return
	}
	inner := tunnel.InnerMTU(maxDg, 0, upperMTU)
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
		RxWorkers:   rxWorkers,
		OnMTULowered: func(m int) {
			if err := tundev.SetMTU(name, m); err != nil {
				log.Printf("re-clamp mtu: %v", err)
			}
		},
	}
	err = tunnel.Run(ctx, dev, conn, cfg)
	log.Printf("tunnel down: %s (%v)", conn.RemoteAddr(), err)
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
