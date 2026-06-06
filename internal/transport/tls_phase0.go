//go:build phase0_insecure

// This file is compiled ONLY when the `phase0_insecure` build tag is set. It
// provides the self-signed certificate and verification-skipping client config
// used by the Phase 0 PoC. Phase 1+ builds omit the tag and get tls_secure.go,
// where these functions refuse to run — so the insecure path cannot be shipped
// by accident.
package transport

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"time"
)

// InsecureBuild reports whether this binary was built with the phase0_insecure tag.
const InsecureBuild = true

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
// verification. Phase 0 ONLY.
func InsecureClientTLS(alpn string) (*tls.Config, error) {
	return &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec // Phase 0 PoC only; gated behind the phase0_insecure build tag.
		NextProtos:         []string{alpn},
		MinVersion:         tls.VersionTLS13,
	}, nil
}
