package server_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/quic-go/quic-go/http3"

	"github.com/lymoos/sieganet/internal/certs"
	"github.com/lymoos/sieganet/internal/server"
)

// startTestServer brings up the server on ephemeral TCP+UDP ports with an
// in-memory CA, and returns the ports plus a root pool that trusts the CA.
func startTestServer(t *testing.T, h http.Handler) (tcpPort, udpPort int, ca *certs.LocalCA) {
	t.Helper()
	ca, err := certs.NewLocalCA("siega.test", "127.0.0.1")
	if err != nil {
		t.Fatalf("local CA: %v", err)
	}
	srv := server.New(ca, h)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	pc, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.ServeTCP(ln) }()
	go func() { _ = srv.ServeQUIC(pc) }()
	t.Cleanup(func() { _ = srv.Close(); _ = ln.Close(); _ = pc.Close() })

	return ln.Addr().(*net.TCPAddr).Port, pc.LocalAddr().(*net.UDPAddr).Port, ca
}

func TestTLSChainOverTCPAndQUIC(t *testing.T) {
	const body = "siega-step1-ok"
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body)
	})
	tcpPort, udpPort, ca := startTestServer(t, h)
	pool := ca.ClientRootPool()

	// --- TCP (HTTP/2) with full chain verification (no InsecureSkipVerify) ---
	tcpClient := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig:   &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS13},
			ForceAttemptHTTP2: true,
		},
	}
	resp, err := tcpClient.Get(fmt.Sprintf("https://127.0.0.1:%d/", tcpPort))
	if err != nil {
		t.Fatalf("tcp GET: %v", err)
	}
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(got) != body {
		t.Errorf("tcp body = %q, want %q", got, body)
	}
	if resp.TLS == nil || !resp.TLS.HandshakeComplete {
		t.Error("tcp TLS handshake not complete")
	}
	if resp.ProtoMajor != 2 {
		t.Errorf("tcp proto = %s, want HTTP/2", resp.Proto)
	}
	t.Logf("TCP: %s, ALPN=%s, verified chain OK", resp.Proto, resp.TLS.NegotiatedProtocol)

	// --- QUIC (HTTP/3) with full chain verification ---
	rt := &http3.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS13}}
	defer rt.Close()
	h3Client := &http.Client{Transport: rt, Timeout: 5 * time.Second}
	// http3 picks the UDP port from the URL authority.
	resp3, err := h3Client.Get(fmt.Sprintf("https://127.0.0.1:%d/", udpPort))
	if err != nil {
		t.Fatalf("h3 GET: %v", err)
	}
	got3, _ := io.ReadAll(resp3.Body)
	resp3.Body.Close()
	if string(got3) != body {
		t.Errorf("h3 body = %q, want %q", got3, body)
	}
	if resp3.ProtoMajor != 3 {
		t.Errorf("h3 proto = %s, want HTTP/3", resp3.Proto)
	}
	if resp3.TLS == nil || !resp3.TLS.HandshakeComplete {
		t.Error("h3 TLS handshake not complete")
	}
	t.Logf("QUIC: %s, verified chain OK", resp3.Proto)
}

// TestUntrustedClientRejected confirms verification is real: a client that does
// not trust the CA must fail the handshake (no InsecureSkipVerify anywhere).
func TestUntrustedClientRejected(t *testing.T) {
	tcpPort, _, _ := startTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	c := &http.Client{Timeout: 5 * time.Second} // default roots, does not trust our CA
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("https://127.0.0.1:%d/", tcpPort), nil)
	if _, err := c.Do(req); err == nil {
		t.Fatal("expected TLS verification failure for untrusted CA, got success")
	}
}
