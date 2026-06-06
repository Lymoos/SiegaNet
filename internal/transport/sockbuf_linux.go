//go:build linux

package transport

import (
	"net"

	"golang.org/x/sys/unix"
)

// readSocketBuffers reports the kernel's actual SO_RCVBUF/SO_SNDBUF for conn.
// The kernel returns twice the value it stored, which is the convention.
func readSocketBuffers(conn *net.UDPConn) (rcv, snd int) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return 0, 0
	}
	_ = raw.Control(func(fd uintptr) {
		rcv, _ = unix.GetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_RCVBUF)
		snd, _ = unix.GetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_SNDBUF)
	})
	return rcv, snd
}
