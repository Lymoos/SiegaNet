package peers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// ClientConfig is everything a client needs to connect, derived from a peer plus
// server-side settings. It renders both as a client.toml and as a compact
// sieganet:// URL suitable for a QR code (Android import).
type ClientConfig struct {
	Server     string `json:"server"` // host:port, e.g. example.com:443
	PeerID     string `json:"peer_id"`
	PSK        string `json:"psk"` // base64 std
	TunnelPath string `json:"tunnel_path"`
	InnerIP    string `json:"inner_ip"` // e.g. 10.7.0.2
	DNS        string `json:"dns"`
	MTU        int    `json:"mtu"`
	KillSwitch bool   `json:"kill_switch"`
}

// NewClientConfig builds a ClientConfig for a peer.
func NewClientConfig(p Peer, server, tunnelPath, dns string, mtu int) ClientConfig {
	return ClientConfig{
		Server:     server,
		PeerID:     p.ID,
		PSK:        base64.StdEncoding.EncodeToString(p.PSK),
		TunnelPath: tunnelPath,
		InnerIP:    p.InnerIP.String(),
		DNS:        dns,
		MTU:        mtu,
		KillSwitch: true,
	}
}

// TOML renders the client configuration file.
func (c ClientConfig) TOML() string {
	var b strings.Builder
	fmt.Fprintf(&b, "server      = %q\n", c.Server)
	fmt.Fprintf(&b, "peer_id     = %q\n", c.PeerID)
	fmt.Fprintf(&b, "psk         = %q\n", c.PSK)
	fmt.Fprintf(&b, "tunnel_path = %q\n", c.TunnelPath)
	fmt.Fprintf(&b, "inner_ip    = %q\n", c.InnerIP)
	fmt.Fprintf(&b, "dns         = %q\n", c.DNS)
	fmt.Fprintf(&b, "mtu         = %d\n", c.MTU)
	fmt.Fprintf(&b, "kill_switch = %t\n", c.KillSwitch)
	return b.String()
}

// URL renders a compact sieganet://<base64url(json)> link for a QR code.
func (c ClientConfig) URL() string {
	j, _ := json.Marshal(c)
	return "sieganet://" + base64.RawURLEncoding.EncodeToString(j)
}

// ParseURL decodes a sieganet:// link back into a ClientConfig (client side).
func ParseURL(u string) (ClientConfig, error) {
	const pfx = "sieganet://"
	if !strings.HasPrefix(u, pfx) {
		return ClientConfig{}, fmt.Errorf("peers: not a sieganet:// url")
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(u, pfx))
	if err != nil {
		return ClientConfig{}, fmt.Errorf("peers: bad url encoding: %w", err)
	}
	var c ClientConfig
	if err := json.Unmarshal(raw, &c); err != nil {
		return ClientConfig{}, fmt.Errorf("peers: bad url payload: %w", err)
	}
	return c, nil
}
