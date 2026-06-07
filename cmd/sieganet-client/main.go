// Command sieganet-client is the Linux SiegaNet client (also the debug client
// for the Windows/Android ports). It opens a WebTransport session to the
// server's magic path, authenticates with its per-peer PSK, brings up a TUN with
// full-tunnel routing and tunnel DNS, and pumps IP packets over the session.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/webtransport-go"

	"github.com/lymoos/sieganet/internal/auth"
	"github.com/lymoos/sieganet/internal/config"
	"github.com/lymoos/sieganet/internal/tundev"
	"github.com/lymoos/sieganet/internal/tunnel"
)

func main() {
	cfgPath := flag.String("config", "client.toml", "path to client config (TOML)")
	caFile := flag.String("ca", "", "trust this CA cert file (default: system roots)")
	tunName := flag.String("tun", "siega0", "TUN interface name")
	endpointDev := flag.String("endpoint-dev", "", "device for the server-endpoint exclusion route (keeps the tunnel off itself)")
	fullTunnel := flag.Bool("full-tunnel", true, "route all traffic through the tunnel (0.0.0.0/1 + 128.0.0.0/1)")
	setDNS := flag.Bool("set-dns", true, "point the system resolver at the tunnel DNS")
	flag.Parse()

	log.SetPrefix("[sieganet-client] ")
	cfg, err := config.LoadClient(*cfgPath)
	if err != nil {
		log.Fatal(err)
	}
	psk, err := base64.StdEncoding.DecodeString(cfg.PSK)
	if err != nil {
		log.Fatalf("psk: %v", err)
	}
	host, _, err := net.SplitHostPort(cfg.Server)
	if err != nil {
		log.Fatalf("server addr: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// TLS trust.
	tlsConf := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS13}
	if *caFile != "" {
		pem, err := os.ReadFile(*caFile)
		if err != nil {
			log.Fatalf("ca: %v", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			log.Fatalf("ca: no certificates in %s", *caFile)
		}
		tlsConf.RootCAs = pool
	}

	// WebTransport dial with the HMAC Authorization header.
	d := &webtransport.Dialer{
		TLSClientConfig: tlsConf,
		QUICConfig:      &quic.Config{EnableDatagrams: true, EnableStreamResetPartialDelivery: true},
	}
	hdr := http.Header{}
	hdr.Set("Authorization", auth.BuildHeader(psk, cfg.PeerID, time.Now()))
	url := "https://" + cfg.Server + cfg.TunnelPath
	dctx, dcancel := context.WithTimeout(ctx, 15*time.Second)
	defer dcancel()
	rsp, sess, err := d.Dial(dctx, url, hdr)
	if err != nil {
		status := 0
		if rsp != nil {
			status = rsp.StatusCode
		}
		log.Fatalf("connect: %v (status %d) — wrong PSK/peer or server down looks like a normal 404", err, status)
	}
	defer sess.CloseWithError(0, "")
	log.Printf("connected to %s as %q", cfg.Server, cfg.PeerID)

	// TUN + addressing.
	dev, err := tundev.Open(*tunName, cfg.MTU)
	if err != nil {
		log.Fatalf("open tun: %v", err)
	}
	defer dev.Close()
	name, _ := tundev.ActualName(dev)
	if err := tundev.ConfigureInterface(name, cfg.InnerIP+"/32"); err != nil {
		log.Fatalf("configure %s: %v", name, err)
	}

	maxDg := probeMaxDatagram(sess)
	inner := tunnel.InnerMTU(maxDg, 0, cfg.MTU)
	_ = tundev.SetMTU(name, inner)
	_ = tundev.ClampMSS(name, inner)
	defer tundev.UnclampMSS(name, inner)
	log.Printf("tun %s up %s, max_datagram=%d inner_mtu=%d", name, cfg.InnerIP, maxDg, inner)

	// Keep the tunnel's own endpoint off the tunnel, then route everything in.
	if srvIP := resolveFirst(host); srvIP != "" && *endpointDev != "" {
		_ = tundev.AddRouteDev(srvIP+"/32", *endpointDev)
	}
	if *fullTunnel {
		_ = tundev.AddRoute(name, "0.0.0.0/1")
		_ = tundev.AddRoute(name, "128.0.0.0/1")
	}
	if *setDNS && cfg.DNS != "" {
		prev, err := tundev.SetResolvConf(cfg.DNS)
		if err != nil {
			log.Printf("set dns: %v", err)
		} else {
			defer tundev.RestoreResolvConf(prev)
			log.Printf("tunnel DNS: %s", cfg.DNS)
		}
	}

	stats := &tunnel.Stats{}
	localIP, _ := netip.ParseAddr(cfg.InnerIP)
	err = tunnel.Run(ctx, dev, sess, tunnel.Config{
		MaxDatagram: maxDg,
		LocalTunIP:  net.IP(localIP.AsSlice()),
		Stats:       stats,
	})
	log.Printf("tunnel down: %v", err)
	log.Print(stats.String())
}

// probeMaxDatagram learns the session's max datagram payload without putting a
// byte on the wire (SendDatagram validates size before queueing), falling back
// to a conservative value if the transport doesn't surface the limit.
func probeMaxDatagram(sess tunnel.Datagrammer) int {
	err := sess.SendDatagram(make([]byte, 65535))
	var tooLarge *quic.DatagramTooLargeError
	if errors.As(err, &tooLarge) {
		return int(tooLarge.MaxDatagramPayloadSize)
	}
	return 1200
}

func resolveFirst(host string) string {
	if ip := net.ParseIP(host); ip != nil {
		return host
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return ""
	}
	return ips[0].String()
}
