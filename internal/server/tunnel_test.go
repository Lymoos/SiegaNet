package server_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/webtransport-go"

	"github.com/lymoos/sieganet/internal/auth"
	"github.com/lymoos/sieganet/internal/certs"
	"github.com/lymoos/sieganet/internal/decoy"
	"github.com/lymoos/sieganet/internal/server"
	"github.com/lymoos/sieganet/internal/tunnel"
)

const (
	testPeer  = "kristina"
	magicPath = "/wt/Sb31x9KQ"
)

var testPSK = []byte("0123456789abcdef0123456789abcdef")

type peerKeys map[string][]byte

func (p peerKeys) PSK(id string) ([]byte, bool) { k, ok := p[id]; return k, ok }

// startTunnelServer brings up the full endpoint (decoy + magic-path tunnel) and
// echoes any datagram a peer sends. Returns the TCP and UDP ports and the CA.
func startTunnelServer(t *testing.T) (tcpPort, udpPort int, ca *certs.LocalCA) {
	t.Helper()
	ca, err := certs.NewLocalCA("siega.test", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	dec, err := decoy.New(decoy.Config{Mode: "static"})
	if err != nil {
		t.Fatal(err)
	}
	srv := server.New(server.Config{
		Cert:      ca,
		Decoy:     dec.Handler(),
		MagicPath: magicPath,
		Auth:      auth.New(peerKeys{testPeer: testPSK}),
		OnSession: func(peerID string, sess tunnel.Datagrammer) {
			// Echo datagrams using only the transport-neutral interface.
			go func() {
				for {
					b, err := sess.ReceiveDatagram(context.Background())
					if err != nil {
						return
					}
					_ = sess.SendDatagram(b)
				}
			}()
		},
	})

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

// dialWT attempts a WebTransport session with the given Authorization header.
// Returns the session (nil on rejection) and the HTTP status code seen.
func dialWT(t *testing.T, udpPort int, ca *certs.LocalCA, authHeader, path string) (*webtransport.Session, int) {
	t.Helper()
	d := &webtransport.Dialer{
		TLSClientConfig: &tls.Config{RootCAs: ca.ClientRootPool(), ServerName: "siega.test", MinVersion: tls.VersionTLS13},
		QUICConfig:      &quic.Config{EnableDatagrams: true, EnableStreamResetPartialDelivery: true},
	}
	hdr := http.Header{}
	if authHeader != "" {
		hdr.Set("Authorization", authHeader)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	rsp, sess, err := d.Dial(ctx, fmt.Sprintf("https://127.0.0.1:%d%s", udpPort, path), hdr)
	status := 0
	if rsp != nil {
		status = rsp.StatusCode
	}
	if err != nil {
		return nil, status
	}
	return sess, status
}

// --- (a) a valid peer opens the tunnel and the datagram channel works ---

func TestValidPeerOpensTunnelAndEchoes(t *testing.T) {
	_, udp, ca := startTunnelServer(t)
	sess, _ := dialWT(t, udp, ca, auth.BuildHeader(testPSK, testPeer, time.Now()), magicPath)
	if sess == nil {
		t.Fatal("valid peer was rejected")
	}
	defer sess.CloseWithError(0, "")

	msg := []byte("ping-through-webtransport")
	if err := sess.SendDatagram(msg); err != nil {
		t.Fatalf("send datagram: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	got, err := sess.ReceiveDatagram(ctx)
	if err != nil {
		t.Fatalf("receive echo: %v", err)
	}
	if string(got) != string(msg) {
		t.Fatalf("echo mismatch: got %q want %q", got, msg)
	}
}

// --- (c) auth edge cases on the magic path: all non-success -> no session ---

func TestAuthEdgeCasesOnMagicPath(t *testing.T) {
	_, udp, ca := startTunnelServer(t)
	now := time.Now()

	// Adjacent minute is within the ±1 window -> MUST succeed.
	if sess, _ := dialWT(t, udp, ca, auth.BuildHeader(testPSK, testPeer, now.Add(-time.Minute)), magicPath); sess != nil {
		sess.CloseWithError(0, "")
	} else {
		t.Error("adjacent-minute token (within window) was rejected")
	}

	reject := []struct {
		name, header string
	}{
		{"expired minute (now-2)", auth.BuildHeader(testPSK, testPeer, now.Add(-2*time.Minute))},
		{"future minute (now+2)", auth.BuildHeader(testPSK, testPeer, now.Add(2*time.Minute))},
		{"correct peer, wrong token", "SiegaNet " + testPeer + ":" + auth.Token([]byte("wrongwrongwrongwrongwrongwrong12"), testPeer, now)},
		{"valid token, unknown peer", auth.BuildHeader(testPSK, "ghost", now)},
		{"no auth header", ""},
		{"malformed header", "SiegaNet not-a-token"},
	}
	for _, c := range reject {
		if sess, status := dialWT(t, udp, ca, c.header, magicPath); sess != nil {
			sess.CloseWithError(0, "")
			t.Errorf("%s: tunnel opened (must be rejected), status=%d", c.name, status)
		}
	}
}

// --- (b) full-response indistinguishability: magic-path bad-auth == generic 404 ---

func TestMagicPathProbeLooksLikeGeneric404(t *testing.T) {
	tcp, _, ca := startTunnelServer(t)
	client := h2Client(ca)

	type resp struct {
		status int
		body   string
		hdr    string // fmt.Sprint of the sorted header map (Go sorts map keys)
	}
	get := func(path, authHeader string) resp {
		req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("https://127.0.0.1:%d%s", tcp, path), nil)
		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}
		r, err := client.Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		b, _ := io.ReadAll(r.Body)
		r.Body.Close()
		h := map[string]string{}
		for k, v := range r.Header {
			if k == "Date" || k == "Alt-Svc" { // regenerated / intentional
				continue
			}
			h[k] = fmt.Sprint(v)
		}
		return resp{r.StatusCode, string(b), fmt.Sprint(h)}
	}

	generic := get("/no-such-path-1234", "")
	magicNoAuth := get(magicPath, "")
	magicBadAuth := get(magicPath, "SiegaNet "+testPeer+":"+auth.Token([]byte("bad-bad-bad-bad-bad-bad-bad-bad1"), testPeer, time.Now()))

	if generic != magicNoAuth {
		t.Errorf("magic-path (no auth) differs from generic 404:\n generic=%+v\n magic=%+v", generic, magicNoAuth)
	}
	if generic != magicBadAuth {
		t.Errorf("magic-path (bad auth) differs from generic 404:\n generic=%+v\n magic=%+v", generic, magicBadAuth)
	}
	if generic.status != http.StatusNotFound {
		t.Errorf("expected 404, got %d", generic.status)
	}
	t.Logf("generic 404 and magic-path probe both: status=%d body=%q", generic.status, generic.body)
}

// --- (timing) does the MAGIC PATH leak timing? ---
//
// The fingerprint question is whether a request to the magic path costs
// differently than the SAME request to a non-magic path. So we hold the request
// (its headers) constant and vary ONLY the URL path. Comparing magic+token vs
// generic+NO-token would instead measure the cost of parsing the Authorization
// header (HPACK + hex decode), which is identical for any path carrying a token
// and so is not a magic-path tell — we report that cohort separately, for
// context, but the pass/fail is on same-header, different-path.
func TestTimingParityMagicVsGeneric(t *testing.T) {
	if testing.Short() {
		t.Skip("timing distribution test skipped in -short")
	}
	tcp, _, ca := startTunnelServer(t)
	client := h2Client(ca)
	badAuth := "SiegaNet " + testPeer + ":" + auth.Token([]byte("bad-bad-bad-bad-bad-bad-bad-bad1"), testPeer, time.Now())

	measure := func(path, authHeader string) time.Duration {
		req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("https://127.0.0.1:%d%s", tcp, path), nil)
		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}
		start := time.Now()
		r, err := client.Do(req)
		if err != nil {
			t.Fatalf("req: %v", err)
		}
		io.Copy(io.Discard, r.Body)
		r.Body.Close()
		return time.Since(start)
	}

	const warmup, n = 300, 4000
	for i := 0; i < warmup; i++ {
		measure("/warm", badAuth)
		measure(magicPath, badAuth)
	}

	// Same header (bad token), different path — isolates path-dependent timing.
	magicTok := make([]time.Duration, 0, n)
	genTok := make([]time.Duration, 0, n)
	// No header on either path — the realistic plain probe.
	magicNo := make([]time.Duration, 0, n)
	genNo := make([]time.Duration, 0, n)
	for i := 0; i < n; i++ { // interleave to cancel drift
		genTok = append(genTok, measure("/no-such-path-1234", badAuth))
		magicTok = append(magicTok, measure(magicPath, badAuth))
		genNo = append(genNo, measure("/no-such-path-1234", ""))
		magicNo = append(magicNo, measure(magicPath, ""))
	}

	mgT, mmT := percentiles(genTok), percentiles(magicTok)
	mgN, mmN := percentiles(genNo), percentiles(magicNo)
	t.Logf("[same bad-token header, vary path]")
	t.Logf("  generic+token : %s", mgT)
	t.Logf("  magic  +token : %s", mmT)
	t.Logf("  delta p50=%+.1fµs p90=%+.1fµs p99=%+.1fµs", usec(mmT.p50-mgT.p50), usec(mmT.p90-mgT.p90), usec(mmT.p99-mgT.p99))
	for _, line := range histogramPair(genTok, magicTok, "generic+token", "magic+token") {
		t.Logf("  %s", line)
	}
	t.Logf("[no header, vary path]")
	t.Logf("  generic       : %s", mgN)
	t.Logf("  magic         : %s", mmN)
	t.Logf("  delta p50=%+.1fµs p90=%+.1fµs p99=%+.1fµs", usec(mmN.p50-mgN.p50), usec(mmN.p90-mgN.p90), usec(mmN.p99-mgN.p99))

	// Pass/fail: holding the request constant, the magic path must not be
	// systematically slower than a generic path at the median. Loopback p50 noise
	// is a few µs; a real path-dependent fingerprint would be stable and larger.
	if d := mmT.p50 - mgT.p50; d > 30*time.Microsecond {
		t.Errorf("magic+token p50 is %.1fµs slower than generic+token — path-dependent timing leak", usec(d))
	}
	if d := mmN.p50 - mgN.p50; d > 30*time.Microsecond {
		t.Errorf("magic p50 is %.1fµs slower than generic — path-dependent timing leak", usec(d))
	}
}

// ---- helpers ----

type pcts struct{ p50, p90, p99, min, max, mean time.Duration }

func (p pcts) String() string {
	return fmt.Sprintf("p50=%6.1fµs p90=%6.1fµs p99=%6.1fµs (min=%.1f max=%.1f mean=%.1f)",
		usec(p.p50), usec(p.p90), usec(p.p99), usec(p.min), usec(p.max), usec(p.mean))
}

func usec(d time.Duration) float64 { return float64(d.Nanoseconds()) / 1000 }

func percentiles(xs []time.Duration) pcts {
	s := append([]time.Duration(nil), xs...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	var sum time.Duration
	for _, v := range s {
		sum += v
	}
	at := func(q float64) time.Duration {
		i := int(q * float64(len(s)-1))
		return s[i]
	}
	return pcts{
		p50: at(0.50), p90: at(0.90), p99: at(0.99),
		min: s[0], max: s[len(s)-1], mean: sum / time.Duration(len(s)),
	}
}

// histogramPair renders two latency samples as side-by-side bucket counts over a
// shared range (p2..p98 of the combined data), so distribution overlap is visible.
func histogramPair(a, b []time.Duration, la, lb string) []string {
	all := append(append([]time.Duration(nil), a...), b...)
	sort.Slice(all, func(i, j int) bool { return all[i] < all[j] })
	lo, hi := all[len(all)*2/100], all[len(all)*98/100]
	const buckets = 12
	width := (hi - lo) / buckets
	if width <= 0 {
		width = 1
	}
	count := func(xs []time.Duration) []int {
		c := make([]int, buckets)
		for _, x := range xs {
			i := int((x - lo) / width)
			if i < 0 {
				i = 0
			}
			if i >= buckets {
				i = buckets - 1
			}
			c[i]++
		}
		return c
	}
	ca, cb := count(a), count(b)
	maxc := 1
	for i := 0; i < buckets; i++ {
		maxc = max1(maxc)
		if ca[i] > maxc {
			maxc = ca[i]
		}
		if cb[i] > maxc {
			maxc = cb[i]
		}
	}
	bar := func(n int) string {
		w := n * 28 / maxc
		s := ""
		for i := 0; i < w; i++ {
			s += "#"
		}
		return s
	}
	out := []string{fmt.Sprintf("histogram (µs≥ → %s | %s), 4000 samples each:", la, lb)}
	for i := 0; i < buckets; i++ {
		b0 := usec(lo + time.Duration(i)*width)
		out = append(out, fmt.Sprintf("  %6.0f %-28s %4d | %-28s %4d", b0, bar(ca[i]), ca[i], bar(cb[i]), cb[i]))
	}
	return out
}

func max1(n int) int {
	if n < 1 {
		return 1
	}
	return n
}

func h2Client(ca *certs.LocalCA) *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig:   &tls.Config{RootCAs: ca.ClientRootPool(), ServerName: "siega.test", MinVersion: tls.VersionTLS13},
			ForceAttemptHTTP2: true,
		},
	}
}
