package decoy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// compareHeaders lists the headers that must match between front and backend
// (Date is regenerated per response, so it is excluded).
var compareHeaders = []string{
	"Content-Type", "Content-Length", "Etag", "Last-Modified", "Accept-Ranges",
}

func fetch(t *testing.T, url string) (*http.Response, []byte) {
	t.Helper()
	resp, err := http.Get(url) //nolint:noctx // test
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, body
}

// TestRelayIsTransparent proves the front relay returns the backend's bytes for
// real pages AND for unknown paths (the 404 comes from the backend FileServer,
// not from SiegaNet code).
func TestRelayIsTransparent(t *testing.T) {
	d, err := New(Config{Mode: "static"}) // embedded site
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	front := httptest.NewServer(d.Handler())
	defer front.Close()

	paths := []string{"/", "/about.html", "/journal.html", "/style.css", "/favicon.svg", "/robots.txt", "/does-not-exist-xyz"}
	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			fResp, fBody := fetch(t, front.URL+p)
			bResp, bBody := fetch(t, d.BackendURL()+p)

			if fResp.StatusCode != bResp.StatusCode {
				t.Errorf("status: front=%d backend=%d", fResp.StatusCode, bResp.StatusCode)
			}
			if string(fBody) != string(bBody) {
				t.Errorf("body differs (front %d bytes, backend %d bytes)", len(fBody), len(bBody))
			}
			for _, h := range compareHeaders {
				if fResp.Header.Get(h) != bResp.Header.Get(h) {
					t.Errorf("header %s: front=%q backend=%q", h, fResp.Header.Get(h), bResp.Header.Get(h))
				}
			}
		})
	}
}

// TestUnknownPathIs404FromBackend documents that the unknown-path response is
// exactly Go's FileServer 404 body, i.e. not a custom SiegaNet error.
func TestUnknownPathIs404FromBackend(t *testing.T) {
	d, err := New(Config{Mode: "static"})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	front := httptest.NewServer(d.Handler())
	defer front.Close()

	resp, body := fetch(t, front.URL+"/nope")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	if got := string(body); got != "404 page not found\n" {
		t.Fatalf("404 body = %q, want the stock FileServer body", got)
	}
}

// TestWeirdRequestsNoPanic sends odd methods and a bad Range and checks the
// relay forwards them to the backend without panicking or emitting a custom
// error (it behaves like the underlying web server).
func TestWeirdRequestsNoPanic(t *testing.T) {
	d, err := New(Config{Mode: "static"})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	front := httptest.NewServer(d.Handler())
	defer front.Close()

	cases := []struct {
		method, path string
		header       http.Header
	}{
		{"FROBNICATE", "/", nil},
		{"GET", "/index.html", http.Header{"Range": {"bytes=99999999-"}}},
		{"DELETE", "/about.html", nil},
	}
	for _, c := range cases {
		req, _ := http.NewRequest(c.method, front.URL+c.path, nil)
		for k, v := range c.header {
			req.Header[k] = v
		}
		fResp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", c.method, c.path, err)
		}
		fResp.Body.Close()

		bReq, _ := http.NewRequest(c.method, d.BackendURL()+c.path, nil)
		for k, v := range c.header {
			bReq.Header[k] = v
		}
		bResp, err := http.DefaultClient.Do(bReq)
		if err != nil {
			t.Fatalf("backend %s %s: %v", c.method, c.path, err)
		}
		bResp.Body.Close()

		if fResp.StatusCode != bResp.StatusCode {
			t.Errorf("%s %s: front=%d backend=%d", c.method, c.path, fResp.StatusCode, bResp.StatusCode)
		}
	}
}
