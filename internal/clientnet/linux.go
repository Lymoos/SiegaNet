//go:build linux

// Package clientnet provides the OS-specific client network configuration
// (clientcore.Configurator). The Linux implementation is the debug/test client:
// it sets up the inner IP, routes and DNS via iproute2 (reusing internal/tundev)
// and treats the kill-switch as a no-op (the kill-switch is a Windows-phase
// feature; the Linux client exists to exercise the data-plane end to end).
package clientnet

import (
	"log"
	"net/netip"

	"golang.zx2c4.com/wireguard/tun"

	"github.com/lymoos/sieganet/internal/clientcore"
	"github.com/lymoos/sieganet/internal/tundev"
)

// Linux configures the tunnel on Linux.
type Linux struct {
	ifaceName string
	innerMTU  int
	prevDNS   []byte
	dnsSet    bool
}

// New returns a Linux Configurator.
func New() *Linux { return &Linux{} }

var _ clientcore.Configurator = (*Linux)(nil)

// Sweep has nothing to do on the Linux debug client: routes and DNS are tied to
// the TUN device, which the kernel removes when the process exits.
func (l *Linux) Sweep() {}

// EngageKillSwitch is a no-op on the Linux debug client.
func (l *Linux) EngageKillSwitch(netip.Addr, string) error {
	log.Printf("[clientnet] kill-switch not implemented on the Linux debug client")
	return nil
}

// ConfigureTUN assigns the inner IP, MTU, routes and DNS.
func (l *Linux) ConfigureTUN(dev tun.Device, p clientcore.TUNParams) error {
	name, err := tundev.ActualName(dev)
	if err != nil {
		return err
	}
	l.ifaceName = name
	l.innerMTU = p.InnerMTU

	if err := tundev.ConfigureInterface(name, p.InnerIP.String()+"/32"); err != nil {
		return err
	}
	_ = tundev.SetMTU(name, p.InnerMTU)
	_ = tundev.ClampMSS(name, p.InnerMTU)

	// Keep the tunnel's own endpoint off the tunnel (anti-loop), then route all.
	if p.EndpointAddr.IsValid() && p.EndpointDev != "" {
		_ = tundev.AddRouteDev(p.EndpointAddr.String()+"/32", p.EndpointDev)
	}
	if p.FullTunnel {
		_ = tundev.AddRoute(name, "0.0.0.0/1")
		_ = tundev.AddRoute(name, "128.0.0.0/1")
	}
	if p.SetDNS && p.DNS != "" {
		if prev, err := tundev.SetResolvConf(p.DNS); err == nil {
			l.prevDNS = prev
			l.dnsSet = true
		} else {
			log.Printf("[clientnet] set dns: %v", err)
		}
	}
	return nil
}

// Cleanup restores DNS and the MSS clamp. Routes/IP go away with the TUN.
func (l *Linux) Cleanup() {
	if l.dnsSet {
		tundev.RestoreResolvConf(l.prevDNS)
		l.dnsSet = false
	}
	if l.ifaceName != "" && l.innerMTU > 0 {
		tundev.UnclampMSS(l.ifaceName, l.innerMTU)
	}
}
