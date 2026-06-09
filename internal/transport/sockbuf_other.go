//go:build !linux

package transport

import "net"

// readSocketBuffers is a no-op outside Linux (the server runs on Linux; this
// keeps the package cross-compilable for tooling).
func readSocketBuffers(*net.UDPConn) (rcv, snd int) { return 0, 0 }
