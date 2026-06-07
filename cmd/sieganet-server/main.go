// Command sieganet-server is the SiegaNet server (Phase 1+).
//
// Step 1: stands up the public endpoint — one handler over TCP (HTTP/1.1+HTTP/2)
// and QUIC (HTTP/3) on port 443 with a real certificate (ACME in production, or
// a file/local-CA source for testing). The decoy site, transparent relay,
// WebTransport tunnel and HMAC peer auth are layered on in the following steps;
// for now the handler is a placeholder.
package main

import (
	"flag"
	"fmt"
	"log"
	"net"

	"github.com/lymoos/sieganet/internal/certs"
	"github.com/lymoos/sieganet/internal/config"
	"github.com/lymoos/sieganet/internal/decoy"
	"github.com/lymoos/sieganet/internal/server"
)

func main() {
	cfgPath := flag.String("config", "configs/server.toml", "path to server config (TOML)")
	flag.Parse()

	log.SetPrefix("[sieganet-server] ")

	cfg, err := config.LoadServer(*cfgPath)
	if err != nil {
		log.Fatal(err)
	}

	src, err := certSource(cfg)
	if err != nil {
		log.Fatalf("certificate source: %v", err)
	}

	// The decoy is the front handler: a transparent relay to a real backend
	// site. Non-tunnel traffic (and, from step 4, failed auth on the magic path)
	// gets the backend's bytes, never a SiegaNet-generated error.
	dec, err := decoy.New(decoy.Config{
		Mode:   cfg.DecoyMode,
		Dir:    cfg.DecoyDir,
		Target: cfg.DecoyTarget,
	})
	if err != nil {
		log.Fatalf("decoy: %v", err)
	}
	defer dec.Close()
	if up := dec.Upstream(); up != "" {
		log.Printf("decoy_mode=proxy relaying to %s", up)
	} else {
		log.Printf("decoy_mode=static serving site (dir=%q, embedded if empty)", cfg.DecoyDir)
	}

	// The tunnel (magic path + auth + session router) is wired in once the peer
	// store lands; until then the endpoint serves only the decoy.
	srv := server.New(server.Config{Cert: src, Decoy: dec.Handler()})

	ln, err := net.Listen("tcp", cfg.ListenTCP)
	if err != nil {
		log.Fatalf("listen tcp %s: %v", cfg.ListenTCP, err)
	}
	udpAddr, err := net.ResolveUDPAddr("udp", cfg.ListenUDP)
	if err != nil {
		log.Fatalf("resolve udp %s: %v", cfg.ListenUDP, err)
	}
	pc, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		log.Fatalf("listen udp %s: %v", cfg.ListenUDP, err)
	}

	log.Printf("cert_mode=%s domain=%s", cfg.CertMode, cfg.Domain)
	log.Printf("serving TCP(h1/h2) on %s and QUIC(h3) on %s", cfg.ListenTCP, cfg.ListenUDP)

	errc := make(chan error, 2)
	go func() { errc <- srv.ServeTCP(ln) }()
	go func() { errc <- srv.ServeQUIC(pc) }()
	log.Fatal(<-errc)
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
		return nil, fmt.Errorf("unknown cert_mode %q", cfg.CertMode) // validated earlier
	}
}
