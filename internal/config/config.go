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

	// Decoy / transparent relay.
	DecoyMode   string `toml:"decoy_mode"`   // "static" (default) | "proxy"
	DecoyDir    string `toml:"decoy_dir"`    // static files; "" => embedded site
	DecoyTarget string `toml:"decoy_target"` // upstream for proxy mode

	// Tunnel / peers (used from step 5 onward).
	TunnelPath       string `toml:"tunnel_path"`     // magic WebTransport path
	InnerSubnet      string `toml:"inner_subnet"`    // e.g. 10.7.0.0/24
	ServerInnerIP    string `toml:"server_inner_ip"` // e.g. 10.7.0.1
	DNS              string `toml:"dns"`             // DNS handed to clients
	PeersStore       string `toml:"peers_store"`     // path to peers.toml
	PadMin           int    `toml:"pad_min"`         // datagram padding range
	PadMax           int    `toml:"pad_max"`
	Obfusc           bool   `toml:"obfusc"`             // Salamander-XOR (Phase 4)
	AllowInterClient bool   `toml:"allow_inter_client"` // client<->client routing
}

// Client is the client-side configuration (client.toml).
type Client struct {
	Server     string `toml:"server"` // host:port
	PeerID     string `toml:"peer_id"`
	PSK        string `toml:"psk"` // base64
	TunnelPath string `toml:"tunnel_path"`
	InnerIP    string `toml:"inner_ip"` // e.g. 10.7.0.2
	DNS        string `toml:"dns"`
	MTU        int    `toml:"mtu"`
	KillSwitch bool   `toml:"kill_switch"`
}

// LoadClient reads a client config file.
func LoadClient(path string) (*Client, error) {
	c := &Client{MTU: 1280}
	if _, err := toml.DecodeFile(path, c); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	if c.Server == "" || c.PeerID == "" || c.PSK == "" || c.TunnelPath == "" {
		return nil, fmt.Errorf("config: server, peer_id, psk and tunnel_path are required")
	}
	return c, nil
}

// LoadServer reads and validates a server config file.
func LoadServer(path string) (*Server, error) {
	c := &Server{
		ListenTCP:     ":443",
		ListenUDP:     ":443",
		CertMode:      "acme",
		InnerSubnet:   "10.7.0.0/24",
		ServerInnerIP: "10.7.0.1",
		DNS:           "1.1.1.1",
		PeersStore:    "./peers.toml",
		PadMax:        256,
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

	if c.DecoyMode == "" {
		c.DecoyMode = "static"
	}
	switch c.DecoyMode {
	case "static":
	case "proxy":
		if c.DecoyTarget == "" {
			return fmt.Errorf("config: decoy_mode=proxy requires decoy_target")
		}
	default:
		return fmt.Errorf("config: unknown decoy_mode %q", c.DecoyMode)
	}
	return nil
}
