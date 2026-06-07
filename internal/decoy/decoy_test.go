package decoy

import (
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// rawGet performs an HTTP/1.1 request over a raw TCP connection and returns the
// full response bytes (status line + headers + body), so header ORDER is
// preserved (the net/http client would parse headers into an unordered map).
// The Date header is blanked because it is regenerated per response.
func rawGet(t *testing.T, addr, path string) string {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial %s: %v", addr, err)
	}
	defer conn.Close()
	fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: siega.test\r\nConnection: close\r\n\r\n", path)
	raw, err := io.ReadAll(conn)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	dateRe := regexp.MustCompile(`(?im)^Date: .*\r?$`)
	return dateRe.ReplaceAllString(string(raw), "Date: <stripped>")
}

func addrOf(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	return strings.TrimPrefix(srv.URL, "http://")
}

// TestStaticIsCanonicalFileServer proves the static decoy front is byte-for-byte
// identical to a stock http.FileServer over the same site — same header set AND
// order, same body, for real pages and the 404. No header appears only on the
// front, so an active prober cannot tell the relay apart from a plain static
// server. (The one intentional addition, Alt-Svc, is added by the server layer,
// not the decoy, and is verified/excluded elsewhere.)
func TestStaticIsCanonicalFileServer(t *testing.T) {
	d, err := New(Config{Mode: "static"})
	if err != nil {
		t.Fatal(err)
	}
	front := httptest.NewServer(d.Handler())
	defer front.Close()

	// Reference: a fresh stdlib FileServer over the same embedded site.
	sub, err := fs.Sub(embeddedSite, "site")
	if err != nil {
		t.Fatal(err)
	}
	ref := httptest.NewServer(http.FileServer(http.FS(sub)))
	defer ref.Close()

	for _, p := range []string{"/", "/about.html", "/journal.html", "/style.css", "/favicon.svg", "/robots.txt", "/nope-404"} {
		t.Run(p, func(t *testing.T) {
			got := rawGet(t, addrOf(t, front), p)
			want := rawGet(t, addrOf(t, ref), p)
			if got != want {
				t.Errorf("front differs from canonical FileServer for %s:\n--- front ---\n%s\n--- reference ---\n%s", p, got, want)
			}
		})
	}
}

// TestProxyTransparent checks that proxy mode forwards to an upstream without
// adding proxy-identifying headers and preserving status/body.
func TestProxyTransparent(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Forwarded-For") != "" || r.Header.Get("Via") != "" {
			t.Errorf("proxy leaked forwarding header: XFF=%q Via=%q",
				r.Header.Get("X-Forwarded-For"), r.Header.Get("Via"))
		}
		w.Header().Set("Content-Type", "text/plain")
		if r.URL.Path == "/missing" {
			http.Error(w, "nope", http.StatusNotFound)
			return
		}
		fmt.Fprintf(w, "backend:%s", r.URL.Path)
	}))
	defer backend.Close()

	d, err := New(Config{Mode: "proxy", Target: backend.URL})
	if err != nil {
		t.Fatal(err)
	}
	front := httptest.NewServer(d.Handler())
	defer front.Close()

	for _, p := range []string{"/", "/page", "/missing"} {
		fr, _ := http.Get(front.URL + p)
		fb, _ := io.ReadAll(fr.Body)
		fr.Body.Close()
		br, _ := http.Get(backend.URL + p)
		bb, _ := io.ReadAll(br.Body)
		br.Body.Close()
		if fr.StatusCode != br.StatusCode {
			t.Errorf("%s: status front=%d backend=%d", p, fr.StatusCode, br.StatusCode)
		}
		if string(fb) != string(bb) {
			t.Errorf("%s: body front=%q backend=%q", p, fb, bb)
		}
	}
}

// TestWeirdRequestsNoPanic sends odd methods and a bad Range to the static
// front and confirms it behaves like the underlying FileServer (no panic).
func TestWeirdRequestsNoPanic(t *testing.T) {
	d, err := New(Config{Mode: "static"})
	if err != nil {
		t.Fatal(err)
	}
	front := httptest.NewServer(d.Handler())
	defer front.Close()

	cases := []struct {
		method, path, rangeHdr string
		wantStatus             int
	}{
		{"FROBNICATE", "/about.html", "", 200},
		{"GET", "/about.html", "bytes=99999999-", 416},
		{"GET", "/about.html", "bytes=0-9", 206},
		{"DELETE", "/about.html", "", 200},
	}
	for _, c := range cases {
		req, _ := http.NewRequest(c.method, front.URL+c.path, nil)
		if c.rangeHdr != "" {
			req.Header.Set("Range", c.rangeHdr)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", c.method, c.path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != c.wantStatus {
			t.Errorf("%s %s range=%q: status=%d want=%d", c.method, c.path, c.rangeHdr, resp.StatusCode, c.wantStatus)
		}
	}
}
