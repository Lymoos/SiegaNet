// Command sieganet-server is the SiegaNet server (Phase 1).
//
// It serves the decoy site over TCP+QUIC, accepts authenticated peers on the
// magic WebTransport path, and routes their traffic through a server TUN with
// per-peer source-IP anti-spoofing and an optional client<->client policy.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/netip"
	"os/signal"
	"syscall"

	"github.com/lymoos/sieganet/internal/auth"
	"github.com/lymoos/sieganet/internal/certs"
	"github.com/lymoos/sieganet/internal/config"
	"github.com/lymoos/sieganet/internal/decoy"
	"github.com/lymoos/sieganet/internal/peers"
	"github.com/lymoos/sieganet/internal/protocol"
	"github.com/lymoos/sieganet/internal/router"
	"github.com/lymoos/sieganet/internal/server"
	"github.com/lymoos/sieganet/internal/tundev"
	"github.com/lymoos/sieganet/internal/tunnel"
)

func main() {
	cfgPath := flag.String("config", "configs/server.toml", "path to server config (TOML)")
	tunName := flag.String("tun", "siega0", "server TUN interface name")
	mtu := flag.Int("mtu", 1280, "server inner MTU")
	egress := flag.String("egress", "", "uplink interface to MASQUERADE through (enables full-tunnel egress)")
	flag.Parse()

	log.SetPrefix("[sieganet-server] ")
	cfg, err := config.LoadServer(*cfgPath)
	if err != nil {
		log.Fatal(err)
	}
	if cfg.TunnelPath == "" {
		log.Fatal("config: tunnel_path is required for the tunnel")
	}

	subnet, err := netip.ParsePrefix(cfg.InnerSubnet)
	if err != nil {
		log.Fatalf("inner_subnet: %v", err)
	}
	serverIP, err := netip.ParseAddr(cfg.ServerInnerIP)
	if err != nil {
		log.Fatalf("server_inner_ip: %v", err)
	}

	src, err := certSource(cfg)
	if err != nil {
		log.Fatalf("certificate source: %v", err)
	}
	store, err := peers.NewFileStore(cfg.PeersStore, subnet, serverIP)
	if err != nil {
		log.Fatalf("peer store: %v", err)
	}
	dec, err := decoy.New(decoy.Config{Mode: cfg.DecoyMode, Dir: cfg.DecoyDir, Target: cfg.DecoyTarget})
	if err != nil {
		log.Fatalf("decoy: %v", err)
	}
	defer dec.Close()

	// Server TUN + forwarding.
	dev, err := tundev.Open(*tunName, *mtu)
	if err != nil {
		log.Fatalf("open tun: %v", err)
	}
	defer dev.Close()
	name, _ := tundev.ActualName(dev)
	cidr := fmt.Sprintf("%s/%d", serverIP, subnet.Bits())
	if err := tundev.ConfigureInterface(name, cidr); err != nil {
		log.Fatalf("configure %s: %v", name, err)
	}
	_ = tundev.SetMTU(name, *mtu)
	_ = tundev.ClampMSS(name, *mtu)
	defer tundev.UnclampMSS(name, *mtu)
	if *egress != "" {
		if err := tundev.EnableIPForward(); err != nil {
			log.Printf("ip_forward: %v", err)
		}
		if err := tundev.AddMasquerade(cfg.InnerSubnet, *egress); err != nil {
			log.Printf("masquerade: %v", err)
		} else {
			defer tundev.DelMasquerade(cfg.InnerSubnet, *egress)
		}
		log.Printf("full-tunnel egress via %s (MASQUERADE %s)", *egress, cfg.InnerSubnet)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	rtr := router.New(dev, serverIP, subnet, cfg.AllowInterClient, protocol.PadRange{Min: cfg.PadMin, Max: cfg.PadMax}, store)
	go func() {
		if err := rtr.Run(ctx); err != nil && ctx.Err() == nil {
			log.Printf("router: %v", err)
		}
	}()

	srv := server.New(server.Config{
		Cert:      src,
		Decoy:     dec.Handler(),
		MagicPath: cfg.TunnelPath,
		Auth:      auth.New(store),
		OnSession: func(peerID string, sess tunnel.Datagrammer) {
			p, ok := store.Lookup(peerID)
			if !ok {
				return
			}
			log.Printf("peer %q connected (%s)", peerID, p.InnerIP)
			rtr.Serve(ctx, peerID, p.InnerIP, sess)
			log.Printf("peer %q disconnected", peerID)
		},
	})

	ln, err := net.Listen("tcp", cfg.ListenTCP)
	if err != nil {
		log.Fatalf("listen tcp %s: %v", cfg.ListenTCP, err)
	}
	udpAddr, _ := net.ResolveUDPAddr("udp", cfg.ListenUDP)
	pc, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		log.Fatalf("listen udp %s: %v", cfg.ListenUDP, err)
	}

	log.Printf("cert_mode=%s domain=%s decoy=%s inner=%s allow_inter_client=%t",
		cfg.CertMode, cfg.Domain, cfg.DecoyMode, cidr, cfg.AllowInterClient)
	log.Printf("serving TCP %s + QUIC %s; tunnel on magic path", cfg.ListenTCP, cfg.ListenUDP)

	errc := make(chan error, 2)
	go func() { errc <- srv.ServeTCP(ln) }()
	go func() { errc <- srv.ServeQUIC(pc) }()
	select {
	case <-ctx.Done():
	case err := <-errc:
		log.Printf("serve: %v", err)
	}
}

func certSource(cfg *config.Server) (certs.Source, error) {
	switch cfg.CertMode {
	case "acme":
		return certs.NewACMESource(cfg.Domain, cfg.ACMECacheDir, cfg.ACMEEmail), nil
	case "file":
		return certs.NewFileSource(cfg.CertFile, cfg.KeyFile)
	case "localca":
		return certs.NewLocalCA(cfg.Domain)
	default:
		return nil, fmt.Errorf("unknown cert_mode %q", cfg.CertMode)
	}
}
