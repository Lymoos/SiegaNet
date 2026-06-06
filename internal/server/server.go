// Package server runs the public-facing endpoint: one HTTP handler served over
// both TCP (HTTP/1.1 + HTTP/2) and QUIC (HTTP/3) on port 443, sharing a single
// certificate from a certs.Source. From step 2 the handler is the decoy /
// transparent relay; the tunnel rides on the same QUIC listener via WebTransport
// on a magic path (step 4). Serving the same site on TCP and UDP makes the
// endpoint look like a normal HTTP/3-capable website to any prober on either
// transport.
package server

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"

	"github.com/lymoos/sieganet/internal/certs"
)

// Server multiplexes one handler over TCP and QUIC.
type Server struct {
	src     certs.Source
	handler http.Handler

	httpSrv *http.Server
	h3Srv   *http3.Server
}

// New builds a Server from a certificate source and a handler.
func New(src certs.Source, handler http.Handler) *Server {
	return &Server{src: src, handler: handler}
}

// tcpTLSConfig advertises HTTP/2 and HTTP/1.1 (plus any ACME challenge ALPN).
func (s *Server) tcpTLSConfig() *tls.Config {
	return &tls.Config{
		GetCertificate: s.src.GetCertificate,
		MinVersion:     tls.VersionTLS13,
		NextProtos:     append([]string{"h2", "http/1.1"}, s.src.ExtraALPN()...),
	}
}

// quicTLSConfig advertises HTTP/3.
func (s *Server) quicTLSConfig() *tls.Config {
	return &tls.Config{
		GetCertificate: s.src.GetCertificate,
		MinVersion:     tls.VersionTLS13,
		NextProtos:     []string{http3.NextProtoH3},
	}
}

// h3 returns (lazily creating) the HTTP/3 server, so handlers can add Alt-Svc
// headers via SetQUICHeaders.
func (s *Server) h3() *http3.Server {
	if s.h3Srv == nil {
		s.h3Srv = &http3.Server{
			Handler:         s.handler,
			TLSConfig:       s.quicTLSConfig(),
			EnableDatagrams: true,
			QUICConfig:      &quic.Config{EnableDatagrams: true},
		}
	}
	return s.h3Srv
}

// AltSvcMiddleware wraps h so TCP responses advertise HTTP/3, like a real H3 site.
func (s *Server) AltSvcMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = s.h3().SetQUICHeaders(w.Header())
		h.ServeHTTP(w, r)
	})
}

// ServeTCP serves HTTP/1.1+HTTP/2 over TLS on ln. Blocks until Close.
func (s *Server) ServeTCP(ln net.Listener) error {
	s.httpSrv = &http.Server{
		Handler:   s.AltSvcMiddleware(s.handler),
		TLSConfig: s.tcpTLSConfig(),
	}
	// Empty cert/key files: ServeTLS uses TLSConfig.GetCertificate and wires up
	// HTTP/2 negotiation.
	return s.httpSrv.ServeTLS(ln, "", "")
}

// ServeQUIC serves HTTP/3 over the given UDP socket. Blocks until Close.
func (s *Server) ServeQUIC(pc net.PacketConn) error {
	return s.h3().Serve(pc)
}

// Close shuts both listeners down.
func (s *Server) Close() error {
	if s.httpSrv != nil {
		_ = s.httpSrv.Shutdown(context.Background())
	}
	if s.h3Srv != nil {
		_ = s.h3Srv.Close()
	}
	return nil
}
