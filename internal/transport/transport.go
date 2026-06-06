// Package transport wraps the QUIC layer used by SiegaNet.
//
// Phase 0 (this file) provides only the bare minimum needed to prove the
// data-plane: a QUIC listener/dialer with DATAGRAM support enabled. The TLS
// identity is a self-signed certificate and the client skips verification —
// this is acceptable ONLY for Phase 0 and is replaced by a real ACME
// certificate + WebTransport masking in Phase 1.
package transport

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"time"

	"github.com/quic-go/quic-go"
)

// ALPNPoC is the ALPN protocol identifier used during Phase 0 (raw QUIC, no
// HTTP/3). Phase 1 switches to "h3".
const ALPNPoC = "sieganet-poc"

// QUICConfig returns the shared quic.Config. DATAGRAM support is mandatory for
// the data-plane (§0.2). Keepalive is enabled to keep NAT bindings alive;
// Phase 1 adds application-level jitter on top.
func QUICConfig() *quic.Config {
	return &quic.Config{
		EnableDatagrams: true,
		KeepAlivePeriod: 15 * time.Second,
		MaxIdleTimeout:  60 * time.Second,
	}
}

// Listen starts a QUIC listener on addr using the given TLS config.
func Listen(addr string, tlsConf *tls.Config) (*quic.Listener, error) {
	return quic.ListenAddr(addr, tlsConf, QUICConfig())
}

// Dial establishes a QUIC connection to addr.
func Dial(ctx context.Context, addr string, tlsConf *tls.Config) (*quic.Conn, error) {
	return quic.DialAddr(ctx, addr, tlsConf, QUICConfig())
}

// SelfSignedTLS generates an in-memory self-signed certificate and returns a
// server TLS config advertising the given ALPN protocol. Phase 0 only.
func SelfSignedTLS(alpn string) (*tls.Config, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "sieganet-poc"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	cert := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{alpn},
		MinVersion:   tls.VersionTLS13,
	}, nil
}

// InsecureClientTLS returns a client TLS config that skips certificate
// verification. Phase 0 ONLY — never used in later phases.
func InsecureClientTLS(alpn string) *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec // Phase 0 PoC only; replaced by real cert verification in Phase 1.
		NextProtos:         []string{alpn},
		MinVersion:         tls.VersionTLS13,
	}
}
