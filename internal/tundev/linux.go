//go:build linux

package tundev

import (
	"fmt"
	"os/exec"

	"golang.zx2c4.com/wireguard/tun"
)

// Open creates a Linux TUN device with the given name and MTU.
func Open(name string, mtu int) (tun.Device, error) {
	dev, err := tun.CreateTUN(name, mtu)
	if err != nil {
		return nil, fmt.Errorf("create tun %q: %w", name, err)
	}
	return dev, nil
}

// ActualName returns the kernel-assigned interface name (CreateTUN may pick a
// different name than requested, e.g. when the requested one is taken).
func ActualName(dev tun.Device) (string, error) {
	return dev.Name()
}

// ConfigureInterface assigns an address and brings the link up via iproute2.
// cidr is in "10.7.0.2/24" form. Phase 0 shells out to `ip`; later phases use
// netlink directly.
func ConfigureInterface(name, cidr string) error {
	if err := run("ip", "addr", "add", cidr, "dev", name); err != nil {
		return err
	}
	if err := run("ip", "link", "set", "dev", name, "up"); err != nil {
		return err
	}
	return nil
}

// AddRoute adds a route for dest (CIDR) over the given interface.
func AddRoute(name, dest string) error {
	return run("ip", "route", "add", dest, "dev", name)
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v: %w: %s", name, args, err, out)
	}
	return nil
}
