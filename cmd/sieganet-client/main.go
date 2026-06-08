// Command sieganet-client is the SiegaNet client (Linux debug client; the
// Windows port shares this entrypoint and the clientcore supervisor). It opens a
// WebTransport session to the server's magic path, authenticates with its
// per-peer PSK, brings up a TUN with full-tunnel routing and tunnel DNS, and
// pumps IP packets over the session via the OS-neutral clientcore supervisor.
//
// The server endpoint is resolved to an IP ONCE here, before clientcore engages
// the kill-switch; every (re)dial uses that cached address, so the client never
// does DNS while its own kill-switch is blocking DNS.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
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
	"github.com/lymoos/sieganet/internal/clientcore"
	"github.com/lymoos/sieganet/internal/clientnet"
	"github.com/lymoos/sieganet/internal/config"
	"github.com/lymoos/sieganet/internal/tundev"
	"github.com/lymoos/sieganet/internal/tunnel"
)

func main() {
	cfgPath := flag.String("config", "client.toml", "path to client config (TOML)")
	caFile := flag.String("ca", "", "trust this CA cert file (default: system roots)")
	tunName := flag.String("tun", "siega0", "TUN interface name")
	endpointDev := flag.String("endpoint-dev", "", "physical device for the server-endpoint exclusion route")
	fullTunnel := flag.Bool("full-tunnel", true, "route all traffic through the tunnel")
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
	host, port, err := net.SplitHostPort(cfg.Server)
	if err != nil {
		log.Fatalf("server addr: %v", err)
	}
	innerIP, err := netip.ParseAddr(cfg.InnerIP)
	if err != nil {
		log.Fatalf("inner_ip: %v", err)
	}

	// Resolve the endpoint to an IP ONCE, before the kill-switch.
	endpointAddr, err := resolveEndpoint(host)
	if err != nil {
		log.Fatalf("resolve %s: %v", host, err)
	}
	log.Printf("server %s -> %s (cached; reconnects never re-resolve under the kill-switch)", host, endpointAddr)

	// TLS trust (SNI = the configured host/domain).
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

	// WebTransport dialer that always connects to the cached endpoint IP.
	endpointHostPort := net.JoinHostPort(endpointAddr.String(), port)
	d := &webtransport.Dialer{
		TLSClientConfig: tlsConf,
		QUICConfig:      &quic.Config{EnableDatagrams: true, EnableStreamResetPartialDelivery: true},
		DialAddr: func(ctx context.Context, _ string, tc *tls.Config, qc *quic.Config) (*quic.Conn, error) {
			return quic.DialAddrEarly(ctx, endpointHostPort, tc, qc)
		},
	}
	url := "https://" + cfg.Server + cfg.TunnelPath

	dial := func(ctx context.Context) (clientcore.Session, error) {
		hdr := http.Header{}
		hdr.Set("Authorization", auth.BuildHeader(psk, cfg.PeerID, time.Now()))
		dctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		rsp, sess, err := d.Dial(dctx, url, hdr)
		if err != nil {
			status := 0
			if rsp != nil {
				status = rsp.StatusCode
			}
			return nil, fmt.Errorf("%w (status %d — wrong PSK/peer looks like a normal 404)", err, status)
		}
		return wtSession{sess}, nil
	}

	stats := &tunnel.Stats{}
	opt := clientcore.Options{
		Configurator: clientnet.New(),
		KillSwitch:   cfg.KillSwitch,
		OpenTUN:      tundev.Open,
		Dial:         dial,
		MaxDatagram:  probeMaxDatagram,
		TunnelConfig: tunnel.Config{Stats: stats, PadMax: 0},
		Log:          log.Printf,
		Params: clientcore.TUNParams{
			TUNName:      *tunName,
			InnerIP:      innerIP,
			InnerMTU:     cfg.MTU,
			DNS:          cfg.DNS,
			SetDNS:       *setDNS,
			FullTunnel:   *fullTunnel,
			EndpointAddr: endpointAddr,
			EndpointPort: port,
			EndpointDev:  *endpointDev,
		},
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	err = opt.Run(ctx)
	log.Printf("client stopped: %v", err)
	log.Print(stats.String())
}

// wtSession adapts a *webtransport.Session to clientcore.Session.
type wtSession struct{ *webtransport.Session }

func (w wtSession) Close() error { return w.CloseWithError(0, "") }

// probeMaxDatagram learns the session's max datagram payload without sending
// anything (SendDatagram validates size before queuing), with a conservative
// fallback for transports that do not surface the limit.
func probeMaxDatagram(s clientcore.Session) int {
	err := s.SendDatagram(make([]byte, 65535))
	var tooLarge *quic.DatagramTooLargeError
	if errors.As(err, &tooLarge) {
		return int(tooLarge.MaxDatagramPayloadSize)
	}
	return 1200
}

func resolveEndpoint(host string) (netip.Addr, error) {
	if ip, err := netip.ParseAddr(host); err == nil {
		return ip, nil
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return netip.Addr{}, err
	}
	for _, ip := range ips {
		if v4 := ip.To4(); v4 != nil {
			a, _ := netip.AddrFromSlice(v4)
			return a, nil
		}
	}
	if len(ips) > 0 {
		a, _ := netip.AddrFromSlice(ips[0])
		return a, nil
	}
	return netip.Addr{}, fmt.Errorf("no addresses")
}
