// Package tundev creates and configures the OS TUN device. The cross-platform
// device implementation comes from golang.zx2c4.com/wireguard/tun (the same
// library used by wireguard-go), so a single read/write code path serves
// Linux, Windows and Android.
//
// The wireguard/tun batch API takes an Offset: callers must leave Offset bytes
// of headroom at the front of every buffer so the Linux backend can prepend its
// virtio-net header in place. We use a fixed Offset shared by the tunnel pump.
package tundev

import "golang.zx2c4.com/wireguard/tun"

// Offset is the headroom (in bytes) reserved at the front of every TUN buffer.
// The Linux backend needs at least virtioNetHdrLen (10) bytes; 16 matches the
// value wireguard-go itself uses and leaves a little slack.
const Offset = 16

// ActualName returns the OS-assigned interface name (CreateTUN may pick a
// different name than requested, e.g. when the requested one is taken).
func ActualName(dev tun.Device) (string, error) {
	return dev.Name()
}
