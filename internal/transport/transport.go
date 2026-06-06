// Package transport wraps the QUIC layer used by SiegaNet.
//
// The QUIC endpoint is built on an explicit *net.UDPConn so we can enlarge the
// kernel socket buffers (SO_RCVBUF/SO_SNDBUF): under high throughput a small
// receive buffer makes the kernel drop UDP packets before quic-go ever sees
// them, which the inner TCP then misreads as congestion. quic-go also tries to
// raise these to its desired size, but doing it ourselves makes the value
// explicit and reportable.
//
// The self-signed/insecure TLS used by Phase 0 lives in tls_phase0.go behind
// the `phase0_insecure` build tag; without that tag the build gets the
// tls_secure.go stubs that refuse to produce an insecure config, so the
// Phase 0 shortcut cannot leak into a later build.
package transport

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/quic-go/quic-go"
)

// ALPNPoC is the ALPN protocol identifier used during Phase 0 (raw QUIC, no
// HTTP/3). Phase 1 switches to "h3".
const ALPNPoC = "sieganet-poc"

// SocketBufferSize is the target size for the UDP socket receive/send buffers.
// Requires net.core.rmem_max/wmem_max to be at least this large (raise via
// sysctl when running as root).
const SocketBufferSize = 16 << 20 // 16 MiB

// QUICConfig returns the shared quic.Config. DATAGRAM support is mandatory for
// the data-plane (§0.2). Keepalive keeps NAT bindings alive; Phase 1 adds
// application-level jitter on top.
func QUICConfig() *quic.Config {
	return &quic.Config{
		EnableDatagrams: true,
		KeepAlivePeriod: 15 * time.Second,
		MaxIdleTimeout:  60 * time.Second,
	}
}

// Endpoint owns a UDP socket and the quic.Transport multiplexed over it.
type Endpoint struct {
	conn      *net.UDPConn
	tr        *quic.Transport
	rcvBuffer int
	sndBuffer int
}

// NewServerEndpoint binds a UDP socket on addr and prepares a QUIC transport.
func NewServerEndpoint(addr string) (*Endpoint, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("resolve %q: %w", addr, err)
	}
	return newEndpoint(udpAddr)
}

// NewClientEndpoint binds an ephemeral UDP socket for dialing.
func NewClientEndpoint() (*Endpoint, error) {
	return newEndpoint(&net.UDPAddr{IP: net.IPv4zero, Port: 0})
}

func newEndpoint(laddr *net.UDPAddr) (*Endpoint, error) {
	conn, err := net.ListenUDP("udp", laddr)
	if err != nil {
		return nil, fmt.Errorf("listen udp: %w", err)
	}
	// Best-effort: enlarge the socket buffers. The kernel silently clamps to
	// net.core.{r,w}mem_max, so we read the values back for reporting.
	_ = conn.SetReadBuffer(SocketBufferSize)
	_ = conn.SetWriteBuffer(SocketBufferSize)
	rcv, snd := readSocketBuffers(conn)
	return &Endpoint{
		conn:      conn,
		tr:        &quic.Transport{Conn: conn},
		rcvBuffer: rcv,
		sndBuffer: snd,
	}, nil
}

// Listen starts accepting QUIC connections with the given TLS config.
func (e *Endpoint) Listen(tlsConf *tls.Config) (*quic.Listener, error) {
	return e.tr.Listen(tlsConf, QUICConfig())
}

// Dial establishes a QUIC connection to addr.
func (e *Endpoint) Dial(ctx context.Context, addr string, tlsConf *tls.Config) (*quic.Conn, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("resolve %q: %w", addr, err)
	}
	return e.tr.Dial(ctx, udpAddr, tlsConf, QUICConfig())
}

// BufferSizes reports the actual kernel socket buffer sizes (bytes). The kernel
// typically returns double the requested value.
func (e *Endpoint) BufferSizes() (rcv, snd int) { return e.rcvBuffer, e.sndBuffer }

// Close releases the transport and socket.
func (e *Endpoint) Close() error {
	_ = e.tr.Close()
	return e.conn.Close()
}

// MaxDatagramSize probes the current maximum DATAGRAM payload size for conn at
// runtime. SendDatagram validates the size and returns DatagramTooLargeError
// *before* queuing anything, so sending an oversized dummy learns the limit
// without putting a single byte on the wire.
func MaxDatagramSize(conn *quic.Conn) (int, error) {
	probe := make([]byte, 65535)
	err := conn.SendDatagram(probe)
	var tooLarge *quic.DatagramTooLargeError
	if errors.As(err, &tooLarge) {
		return int(tooLarge.MaxDatagramPayloadSize), nil
	}
	if err != nil {
		return 0, err
	}
	// A 64 KiB datagram fit (impossible over a real path); treat as unbounded.
	return 65535, nil
}
