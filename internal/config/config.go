// Package config loads the SiegaNet server configuration from TOML. Fields are
// added as each Phase 1 step needs them; step 1 only requires the listeners and
// the certificate source.
package config

import (
	"fmt"

	"github.com/BurntSushi/toml"
)

// Server is the server-side configuration.
type Server struct {
	Domain    string `toml:"domain"`
	ListenTCP string `toml:"listen_tcp"`
	ListenUDP string `toml:"listen_udp"`

	// Certificate source: "acme" (Let's Encrypt), "file" (cert_file/key_file),
	// or "localca" (in-memory, dev only).
	CertMode     string `toml:"cert_mode"`
	CertFile     string `toml:"cert_file"`
	KeyFile      string `toml:"key_file"`
	ACMECacheDir string `toml:"acme_cache_dir"`
	ACMEEmail    string `toml:"acme_email"`
}

// LoadServer reads and validates a server config file.
func LoadServer(path string) (*Server, error) {
	c := &Server{
		ListenTCP: ":443",
		ListenUDP: ":443",
		CertMode:  "acme",
	}
	if _, err := toml.DecodeFile(path, c); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	if err := c.validate(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Server) validate() error {
	switch c.CertMode {
	case "acme":
		if c.Domain == "" {
			return fmt.Errorf("config: cert_mode=acme requires domain")
		}
	case "file":
		if c.CertFile == "" || c.KeyFile == "" {
			return fmt.Errorf("config: cert_mode=file requires cert_file and key_file")
		}
	case "localca":
		if c.Domain == "" {
			c.Domain = "localhost"
		}
	default:
		return fmt.Errorf("config: unknown cert_mode %q", c.CertMode)
	}
	return nil
}
