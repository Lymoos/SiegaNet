//go:build linux

package tundev

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

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

// SetMTU sets the interface MTU at runtime. The data-plane calls this after it
// has probed the actual maximum QUIC datagram size, so the inner MTU is clamped
// to the path rather than a baked-in constant (§3.4 MTU gate).
func SetMTU(name string, mtu int) error {
	return run("ip", "link", "set", "dev", name, "mtu", strconv.Itoa(mtu))
}

// ClampMSS installs TCPMSS rules pinning the SYN MSS of TCP that leaves over
// the tunnel interface to mtu-40 (IPv4 header 20 + TCP header 20). This is what
// keeps forwarded TCP (Phase 1 MASQUERADE) from emitting segments that won't
// fit a datagram; for locally-originated TCP the kernel already derives the MSS
// from the interface MTU, but the explicit rule makes the behaviour uniform.
func ClampMSS(name string, mtu int) error {
	mss := mtu - 40
	for _, chain := range []string{"FORWARD", "OUTPUT"} {
		args := []string{"-t", "mangle", "-A", chain, "-o", name,
			"-p", "tcp", "--tcp-flags", "SYN,RST", "SYN",
			"-j", "TCPMSS", "--set-mss", strconv.Itoa(mss)}
		if err := run("iptables", args...); err != nil {
			return err
		}
	}
	return nil
}

// UnclampMSS removes the rules installed by ClampMSS (best-effort).
func UnclampMSS(name string, mtu int) {
	mss := mtu - 40
	for _, chain := range []string{"FORWARD", "OUTPUT"} {
		_ = run("iptables", "-t", "mangle", "-D", chain, "-o", name,
			"-p", "tcp", "--tcp-flags", "SYN,RST", "SYN",
			"-j", "TCPMSS", "--set-mss", strconv.Itoa(mss))
	}
}

// IfaceDrops reads the kernel rx_dropped/tx_dropped counters for an interface.
// tx_dropped on a TUN rises when the kernel queues packets to userspace faster
// than the data-plane reads them (send-side backpressure / pacing).
func IfaceDrops(name string) (rxDropped, txDropped uint64, err error) {
	rxDropped, err = readStat(name, "rx_dropped")
	if err != nil {
		return 0, 0, err
	}
	txDropped, err = readStat(name, "tx_dropped")
	return rxDropped, txDropped, err
}

func readStat(name, stat string) (uint64, error) {
	b, err := os.ReadFile(fmt.Sprintf("/sys/class/net/%s/statistics/%s", name, stat))
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v: %w: %s", name, args, err, out)
	}
	return nil
}
