// Package certs provides the server's TLS certificate from a pluggable source:
//
//   - LocalCA: an in-memory CA + leaf, for tests (real verification, no
//     InsecureSkipVerify — the test client trusts the CA).
//   - File: a cert/key pair loaded from disk.
//   - ACME: a Let's Encrypt certificate via autocert (TLS-ALPN-01), for prod.
//
// A Source only needs to answer GetCertificate (used in tls.Config) and declare
// any extra ALPN protocols its challenge needs (acme-tls/1 for ACME). The server
// builds separate tls.Configs for the TCP (h2/http1.1) and QUIC (h3) listeners
// that share the same certificate.
package certs

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net"
	"time"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

// Source supplies the server certificate during the TLS handshake.
type Source interface {
	// GetCertificate returns the certificate for a ClientHello (SNI-aware).
	GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error)
	// ExtraALPN lists ALPN protocols the source must handle on the TCP listener
	// (e.g. "acme-tls/1" for ACME TLS-ALPN-01). May be nil.
	ExtraALPN() []string
}

// ---- LocalCA: in-memory CA + leaf, for tests ----

// LocalCA is a self-contained certificate authority that issues a single leaf
// for a domain. Tests trust ClientRootPool() so the full verification path runs
// without InsecureSkipVerify.
type LocalCA struct {
	caCert *x509.Certificate
	leaf   tls.Certificate
}

// NewLocalCA creates a CA and issues a leaf valid for the given domain(s).
func NewLocalCA(domains ...string) (*LocalCA, error) {
	if len(domains) == 0 {
		return nil, fmt.Errorf("certs: NewLocalCA needs at least one domain")
	}
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "SiegaNet Test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, err
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		return nil, err
	}

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: domains[0]},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(90 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	for _, d := range domains {
		if ip := net.ParseIP(d); ip != nil {
			leafTmpl.IPAddresses = append(leafTmpl.IPAddresses, ip)
		} else {
			leafTmpl.DNSNames = append(leafTmpl.DNSNames, d)
		}
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		return nil, err
	}
	return &LocalCA{
		caCert: caCert,
		leaf: tls.Certificate{
			Certificate: [][]byte{leafDER},
			PrivateKey:  leafKey,
			Leaf:        mustParse(leafDER),
		},
	}, nil
}

func mustParse(der []byte) *x509.Certificate {
	c, _ := x509.ParseCertificate(der)
	return c
}

func (l *LocalCA) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return &l.leaf, nil
}

func (l *LocalCA) ExtraALPN() []string { return nil }

// ClientRootPool returns a pool trusting this CA, for test clients.
func (l *LocalCA) ClientRootPool() *x509.CertPool {
	p := x509.NewCertPool()
	p.AddCert(l.caCert)
	return p
}

// CACertPEM / leaf accessors are provided for writing test fixtures to disk.
func (l *LocalCA) CACert() *x509.Certificate { return l.caCert }

// ---- File: cert/key from disk ----

// FileSource serves a certificate loaded from PEM files.
type FileSource struct{ cert tls.Certificate }

// NewFileSource loads a certificate and key pair.
func NewFileSource(certFile, keyFile string) (*FileSource, error) {
	c, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("certs: load key pair: %w", err)
	}
	return &FileSource{cert: c}, nil
}

func (f *FileSource) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return &f.cert, nil
}

func (f *FileSource) ExtraALPN() []string { return nil }

// ---- ACME: Let's Encrypt via autocert (production) ----

// ACMESource obtains certificates from an ACME CA using TLS-ALPN-01. The TCP
// listener must advertise the acme-tls/1 ALPN (see ExtraALPN) so the CA's
// challenge handshakes reach autocert.
type ACMESource struct{ m *autocert.Manager }

// NewACMESource configures autocert for the given domain, caching certs in dir.
func NewACMESource(domain, cacheDir, email string) *ACMESource {
	return &ACMESource{m: &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(domain),
		Cache:      autocert.DirCache(cacheDir),
		Email:      email,
	}}
}

func (a *ACMESource) GetCertificate(h *tls.ClientHelloInfo) (*tls.Certificate, error) {
	return a.m.GetCertificate(h)
}

func (a *ACMESource) ExtraALPN() []string { return []string{acme.ALPNProto} }
