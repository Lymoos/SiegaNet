// Package server runs the public-facing endpoint: one HTTP handler served over
// both TCP (HTTP/1.1 + HTTP/2) and QUIC (HTTP/3) on port 443, sharing one
// certificate from a certs.Source. The tunnel rides on the same QUIC listener as
// a WebTransport session on a secret "magic" path, reachable only after HMAC
// peer authentication; everything else — and every failed authentication — is
// served by the decoy handler, so the endpoint is indistinguishable from a plain
// website to any prober on either transport.
//
// Stealth-critical detail: peer authentication is computed on EVERY request
// (constant work) and its result is only consulted for an actual WebTransport
// upgrade. An unauthenticated request never even compares the URL path against
// the magic path, so probing the magic path is byte- and timing-identical to
// requesting any other non-existent path.
package server

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/webtransport-go"

	"github.com/lymoos/sieganet/internal/auth"
	"github.com/lymoos/sieganet/internal/certs"
	"github.com/lymoos/sieganet/internal/tunnel"
)

// SessionFunc is invoked when an authenticated peer opens the tunnel. The
// session is handed over as a transport-neutral tunnel.Datagrammer, so the
// session router and data-plane never see WebTransport/QUIC specifics.
type SessionFunc func(peerID string, sess tunnel.Datagrammer)

// Config configures the endpoint.
type Config struct {
	Cert  certs.Source // certificate source (required)
	Decoy http.Handler // handler for all non-tunnel traffic (required)

	// MagicPath, Auth and OnSession enable the tunnel. If MagicPath is empty the
	// endpoint serves only the decoy (used by the early Phase 1 steps).
	MagicPath string
	Auth      *auth.Authenticator
	OnSession SessionFunc
}

// Server multiplexes the endpoint over TCP and QUIC.
type Server struct {
	cfg     Config
	wt      *webtransport.Server
	httpSrv *http.Server
}

// New builds the endpoint.
func New(cfg Config) *Server {
	s := &Server{cfg: cfg}
	s.wt = &webtransport.Server{
		H3: &http3.Server{
			TLSConfig:       s.quicTLSConfig(),
			EnableDatagrams: true,
			QUICConfig: &quic.Config{
				EnableDatagrams:                  true,
				EnableStreamResetPartialDelivery: true,
			},
		},
		// We authenticate peers with HMAC, so the WebTransport origin check is
		// not our security boundary; allow all origins.
		CheckOrigin: func(*http.Request) bool { return true },
	}
	// Advertise the WebTransport/Extended-CONNECT/datagram SETTINGS and inject
	// the QUIC connection into the request context (required by Upgrade).
	webtransport.ConfigureHTTP3Server(s.wt.H3)
	s.wt.H3.Handler = s.topHandler()
	return s
}

func (s *Server) tcpTLSConfig() *tls.Config {
	return &tls.Config{
		GetCertificate: s.cfg.Cert.GetCertificate,
		MinVersion:     tls.VersionTLS13,
		NextProtos:     append([]string{"h2", "http/1.1"}, s.cfg.Cert.ExtraALPN()...),
	}
}

func (s *Server) quicTLSConfig() *tls.Config {
	return &tls.Config{
		GetCertificate: s.cfg.Cert.GetCertificate,
		MinVersion:     tls.VersionTLS13,
		NextProtos:     []string{http3.NextProtoH3},
	}
}

// isWebTransport reports whether r is a WebTransport CONNECT request.
func isWebTransport(r *http.Request) bool {
	return r.Method == http.MethodConnect && r.Proto == "webtransport"
}

// topHandler is the shared handler for both transports.
func (s *Server) topHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Constant-work authentication on every request (timing parity). The
		// short-circuit on !authed means an unauthenticated request never even
		// compares the path, so magic-path probing == any other 404.
		authed := false
		var peerID string
		if s.cfg.Auth != nil {
			peerID, authed = s.cfg.Auth.Authorize(r.Header.Get("Authorization"))
		}
		if authed && s.cfg.MagicPath != "" && isWebTransport(r) && r.URL.Path == s.cfg.MagicPath {
			sess, err := s.wt.Upgrade(w, r)
			if err != nil {
				return
			}
			s.cfg.OnSession(peerID, sess)
			return
		}
		s.cfg.Decoy.ServeHTTP(w, r)
	})
}

// altSvc wraps h so TCP responses advertise HTTP/3, like a real H3 site.
func (s *Server) altSvc(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = s.wt.H3.SetQUICHeaders(w.Header())
		h.ServeHTTP(w, r)
	})
}

// ServeTCP serves HTTP/1.1+HTTP/2 over TLS on ln. Blocks until Close.
func (s *Server) ServeTCP(ln net.Listener) error {
	s.httpSrv = &http.Server{
		Handler:   s.altSvc(s.topHandler()),
		TLSConfig: s.tcpTLSConfig(),
	}
	return s.httpSrv.ServeTLS(ln, "", "")
}

// ServeQUIC serves HTTP/3 (and WebTransport) over the UDP socket. Blocks.
func (s *Server) ServeQUIC(pc net.PacketConn) error {
	return s.wt.Serve(pc)
}

// Close shuts both listeners down.
func (s *Server) Close() error {
	if s.httpSrv != nil {
		_ = s.httpSrv.Shutdown(context.Background())
	}
	if s.wt != nil {
		_ = s.wt.Close()
	}
	return nil
}
