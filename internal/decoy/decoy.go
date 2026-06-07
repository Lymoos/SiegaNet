// Package decoy serves the cover website and, in proxy mode, a transparent relay
// in front of an upstream. Following the Reality approach, the public endpoint
// does not generate its own 404s or error pages: every non-tunnel request (and,
// from step 4, every failed-auth request to the magic path) is handled by the
// decoy, so a prober receives byte-for-byte the response of a genuine website.
//
//   - static mode (default): the embedded (or on-disk) site is served directly
//     by the standard library's http.FileServer. The front therefore produces
//     byte-identical, canonical Go responses — including header order and the
//     stock 404 — with no reverse-proxy artifacts.
//   - proxy mode: requests are transparently reverse-proxied to an external
//     upstream URL (preserving method/path/body/status/headers, adding no
//     X-Forwarded-For / Via / proxy-identifying header).
package decoy

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
)

//go:embed site
var embeddedSite embed.FS

// Config selects the decoy behaviour.
type Config struct {
	Mode   string // "static" (default) | "proxy"
	Dir    string // static files dir; "" uses the embedded default site
	Target string // upstream URL for proxy mode
}

// Decoy holds the front handler.
type Decoy struct {
	handler  http.Handler
	upstream string // "" for static; the target URL for proxy
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
	// Serve the FileServer directly: the front IS a stock Go static server, so
	// an active prober sees canonical responses with no proxy fingerprint.
	return &Decoy{handler: http.FileServer(http.FS(fsys))}, nil
}

func newProxy(cfg Config) (*Decoy, error) {
	if cfg.Target == "" {
		return nil, fmt.Errorf("decoy: proxy mode requires target")
	}
	target, err := url.Parse(cfg.Target)
	if err != nil {
		return nil, fmt.Errorf("decoy: bad target %q: %w", cfg.Target, err)
	}
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)         // route to upstream; preserves path/query
			pr.Out.Host = target.Host // present the upstream's host
			// Deliberately no pr.SetXForwarded(): no X-Forwarded-For / Via leak.
		},
	}
	return &Decoy{handler: proxy, upstream: cfg.Target}, nil
}

// Handler returns the front handler (static FileServer or transparent relay).
func (d *Decoy) Handler() http.Handler { return d.handler }

// Upstream is the proxied upstream URL, or "" in static mode (diagnostics).
func (d *Decoy) Upstream() string { return d.upstream }

// Close releases resources (none currently; kept for API stability).
func (d *Decoy) Close() error { return nil }
