// Package decoy serves the cover website and the transparent relay in front of
// it. Following the Reality approach, the public endpoint does not generate its
// own 404s or error pages: every non-tunnel request (and, from step 4, every
// failed-auth request to the magic path) is reverse-proxied to a real backend,
// so a prober receives byte-for-byte the response of a genuine website.
//
//   - static mode: a real http.FileServer backend is started on localhost
//     serving an embedded (or on-disk) site; the front proxies to it.
//   - proxy mode: the front proxies straight to an external upstream URL.
//
// The proxy is transparent: it preserves method, path, query, body, status and
// the backend's headers, and does NOT add X-Forwarded-For / Via or any
// proxy-identifying header.
package decoy

import (
	"embed"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
)

//go:embed site
var embeddedSite embed.FS

// Config selects the decoy behaviour.
type Config struct {
	Mode          string // "static" (default) | "proxy"
	Dir           string // static files dir; "" uses the embedded default site
	Target        string // upstream URL for proxy mode
	BackendListen string // static backend bind addr; "" => 127.0.0.1:0
}

// Decoy holds the front handler and, in static mode, the local backend server.
type Decoy struct {
	handler    http.Handler
	backend    *http.Server
	backendURL string
}

// New builds a Decoy from cfg.
func New(cfg Config) (*Decoy, error) {
	switch cfg.Mode {
	case "", "static":
		return newStatic(cfg)
	case "proxy":
		return newProxy(cfg)
	default:
		return nil, fmt.Errorf("decoy: unknown mode %q", cfg.Mode)
	}
}

func newStatic(cfg Config) (*Decoy, error) {
	var fsys fs.FS
	if cfg.Dir != "" {
		fsys = os.DirFS(cfg.Dir)
	} else {
		sub, err := fs.Sub(embeddedSite, "site")
		if err != nil {
			return nil, err
		}
		fsys = sub
	}

	listen := cfg.BackendListen
	if listen == "" {
		listen = "127.0.0.1:0"
	}
	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return nil, fmt.Errorf("decoy backend listen %q: %w", listen, err)
	}
	backend := &http.Server{Handler: http.FileServer(http.FS(fsys))}
	go func() { _ = backend.Serve(ln) }()

	backendURL := "http://" + ln.Addr().String()
	target, err := url.Parse(backendURL)
	if err != nil {
		return nil, err
	}
	return &Decoy{
		handler:    transparentProxy(target, true),
		backend:    backend,
		backendURL: backendURL,
	}, nil
}

func newProxy(cfg Config) (*Decoy, error) {
	if cfg.Target == "" {
		return nil, fmt.Errorf("decoy: proxy mode requires target")
	}
	target, err := url.Parse(cfg.Target)
	if err != nil {
		return nil, fmt.Errorf("decoy: bad target %q: %w", cfg.Target, err)
	}
	return &Decoy{
		handler:    transparentProxy(target, false),
		backendURL: cfg.Target,
	}, nil
}

// transparentProxy builds a reverse proxy to target. When preserveHost is true
// the client's Host header is forwarded unchanged (local backend); otherwise the
// upstream's host is used (external site).
func transparentProxy(target *url.URL, preserveHost bool) http.Handler {
	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target) // route to backend; preserves the request path/query
			if preserveHost {
				pr.Out.Host = pr.In.Host
			} else {
				pr.Out.Host = target.Host
			}
			// Deliberately do NOT call pr.SetXForwarded(): no X-Forwarded-For /
			// X-Forwarded-Host / Via headers leak that this is a proxy.
		},
	}
}

// Handler returns the front handler (the transparent relay).
func (d *Decoy) Handler() http.Handler { return d.handler }

// BackendURL is the upstream the relay forwards to (for tests/diagnostics).
func (d *Decoy) BackendURL() string { return d.backendURL }

// Close stops the local backend (static mode).
func (d *Decoy) Close() error {
	if d.backend != nil {
		return d.backend.Close()
	}
	return nil
}
