// Package tunnel pumps IP packets between a TUN device and a QUIC connection's
// DATAGRAM channel. One IP packet maps to exactly one QUIC DATAGRAM (§0.2).
//
// Goroutines per tunnel:
//
//	outbound: TUN.Read -> gate/encode -> conn.SendDatagram
//	inbound:  conn.ReceiveDatagram -> decode -> writeCh
//	writer:   writeCh -> TUN.Write          (single writer; also injects ICMP)
//
// All TUN writes funnel through one goroutine so the inbound path and the
// outbound path's ICMP injection never call TUN.Write concurrently.
//
// MTU gate (§3.4): the largest IP packet the kernel hands us equals the TUN
// MTU, which the caller sets from InnerMTU(maxDatagram,...). Each framed
// datagram is still checked against the runtime gate; an oversized IPv4 packet
// with DF set is answered with an ICMP "fragmentation needed" instead of being
// dropped silently, so the sender's PMTU discovery converges rather than
// blackholing.
package tunnel

import (
	"context"
	"errors"
	"log"
	"net"
	"sync"
	"sync/atomic"

	"github.com/quic-go/quic-go"
	"golang.zx2c4.com/wireguard/tun"

	"github.com/lymoos/sieganet/internal/protocol"
	"github.com/lymoos/sieganet/internal/tundev"
)

const (
	maxPacket    = 65535
	writeBacklog = 2048 // TUN-write queue depth; relieves quic-go's 128-deep rcv queue
)

// Datagrammer is the unreliable-datagram transport the data-plane rides on. It
// is deliberately the minimal surface shared by a raw *quic.Conn (Phase 0) and a
// *webtransport.Session (Phase 1+), so the pump, the session router and the auth
// layer carry no QUIC-specific assumptions and a second transport can be added
// later without touching the core (the swappable-transport requirement).
type Datagrammer interface {
	SendDatagram([]byte) error
	ReceiveDatagram(context.Context) ([]byte, error)
}

// Config parameterises a tunnel run.
type Config struct {
	Pad         protocol.PadRange
	MaxDatagram int    // runtime-probed max DATAGRAM payload (the initial gate)
	PadMax      int    // upper bound of Pad, for the MTU/ICMP computation
	LocalTunIP  net.IP // source address used for injected ICMP errors
	Stats       *Stats
	// OnMTULowered, if set, is called when the live gate shrinks below the
	// startup value (e.g. path MTU dropped) with the new inner MTU, so the
	// caller can re-clamp the TUN interface MTU.
	OnMTULowered func(innerMTU int)
	// RxWorkers is the number of goroutines draining the QUIC datagram receive
	// queue. quic-go's receive queue is only 128 deep and is not configurable;
	// at line-rate floods (>1 Gbit/s) a single consumer can let it overflow,
	// which shows up as datagram loss the inner TCP must retransmit. More
	// workers drain it faster at the cost of possible packet reordering, so the
	// default is 1 (ordering-safe); raise it for high-bandwidth links where the
	// peers are well-connected. Values < 1 mean 1.
	RxWorkers int
}

type pooledPkt struct {
	buf []byte // has tundev.Offset bytes of headroom; packet at buf[Offset:Offset+n]
	n   int
}

// Run drives the data-plane until the connection or context ends.
func Run(ctx context.Context, dev tun.Device, tr Datagrammer, cfg Config) error {
	if cfg.Stats == nil {
		cfg.Stats = &Stats{}
	}
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	gate := &atomic.Int64{}
	gate.Store(int64(cfg.MaxDatagram))

	pool := &sync.Pool{New: func() any {
		return &pooledPkt{buf: make([]byte, tundev.Offset+maxPacket)}
	}}
	writeCh := make(chan *pooledPkt, writeBacklog)

	rxWorkers := cfg.RxWorkers
	if rxWorkers < 1 {
		rxWorkers = 1
	}

	errc := make(chan error, 1+rxWorkers)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); tunWriter(cctx, dev, writeCh, pool, cfg.Stats) }()

	go func() { errc <- pumpOutbound(dev, tr, cfg, gate, writeCh, pool) }()
	for i := 0; i < rxWorkers; i++ {
		go func() { errc <- pumpInbound(cctx, tr, cfg.Stats, writeCh, pool) }()
	}

	var err error
	select {
	case <-ctx.Done():
		err = ctx.Err()
	case err = <-errc:
	}
	cancel()
	wg.Wait()
	return err
}

// enqueueWrite copies pkt into a pooled buffer and queues it for the TUN
// writer. Drops (and counts) if the backlog is full.
func enqueueWrite(pkt []byte, writeCh chan *pooledPkt, pool *sync.Pool, dropCtr *atomic.Uint64) {
	p := pool.Get().(*pooledPkt)
	p.n = copy(p.buf[tundev.Offset:], pkt)
	select {
	case writeCh <- p:
	default:
		pool.Put(p)
		dropCtr.Add(1)
	}
}

// pumpOutbound reads packets from the TUN and sends each as a DATAGRAM.
func pumpOutbound(dev tun.Device, tr Datagrammer, cfg Config, gate *atomic.Int64, writeCh chan *pooledPkt, pool *sync.Pool) error {
	st := cfg.Stats
	batch := dev.BatchSize()
	bufs := make([][]byte, batch)
	sizes := make([]int, batch)
	for i := range bufs {
		bufs[i] = make([]byte, tundev.Offset+maxPacket)
	}
	var sendBuf []byte

	handleOversize := func(pkt []byte) {
		st.TxDropOversize.Add(1)
		if isIPv4(pkt) && ipv4HasDF(pkt) && cfg.LocalTunIP != nil {
			innerMTU := InnerMTU(int(gate.Load()), cfg.PadMax, 0)
			if icmp := buildICMPv4FragNeeded(pkt, cfg.LocalTunIP, innerMTU); icmp != nil {
				enqueueWrite(icmp, writeCh, pool, &st.RxDropBacklog)
			}
		}
	}

	for {
		n, err := dev.Read(bufs, sizes, tundev.Offset)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}
		for i := 0; i < n; i++ {
			pkt := bufs[i][tundev.Offset : tundev.Offset+sizes[i]]
			if len(pkt) == 0 {
				continue
			}
			st.TxTUN.Add(1)

			sendBuf, err = protocol.EncodeDatagram(sendBuf, pkt, cfg.Pad)
			if err != nil {
				return err
			}
			// Proactive MTU gate: never even offer an oversized datagram.
			if int64(len(sendBuf)) > gate.Load() {
				handleOversize(pkt)
				continue
			}
			if err := tr.SendDatagram(sendBuf); err != nil {
				var tooLarge *quic.DatagramTooLargeError
				if errors.As(err, &tooLarge) {
					// Path MTU shrank: lower the gate and tell the caller so it
					// can re-clamp the interface MTU.
					lowerGate(gate, tooLarge.MaxDatagramPayloadSize, cfg)
					handleOversize(pkt)
					continue
				}
				st.TxDropErr.Add(1)
				return err
			}
			st.TxSent.Add(1)
		}
	}
}

// lowerGate atomically reduces the gate and notifies the caller if it actually
// dropped below the current value.
func lowerGate(gate *atomic.Int64, newMax int64, cfg Config) {
	for {
		cur := gate.Load()
		if newMax >= cur {
			return
		}
		if gate.CompareAndSwap(cur, newMax) {
			log.Printf("tunnel: max datagram lowered %d -> %d", cur, newMax)
			if cfg.OnMTULowered != nil {
				cfg.OnMTULowered(InnerMTU(int(newMax), cfg.PadMax, 0))
			}
			return
		}
	}
}

// pumpInbound receives DATAGRAMs and queues the carried IP packet for the writer.
func pumpInbound(ctx context.Context, tr Datagrammer, st *Stats, writeCh chan *pooledPkt, pool *sync.Pool) error {
	for {
		dg, err := tr.ReceiveDatagram(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}
		st.RxRecv.Add(1)
		pkt, err := protocol.DecodeDatagram(dg)
		if err != nil || len(pkt) == 0 {
			st.RxDropMalformed.Add(1)
			continue
		}
		enqueueWrite(pkt, writeCh, pool, &st.RxDropBacklog)
	}
}

// tunWriter is the sole writer to the TUN device.
func tunWriter(ctx context.Context, dev tun.Device, writeCh chan *pooledPkt, pool *sync.Pool, st *Stats) {
	wb := make([][]byte, 1)
	for {
		select {
		case <-ctx.Done():
			return
		case p := <-writeCh:
			wb[0] = p.buf[:tundev.Offset+p.n]
			if _, err := dev.Write(wb, tundev.Offset); err != nil {
				st.RxDropTun.Add(1)
			} else {
				st.RxWritten.Add(1)
			}
			pool.Put(p)
		}
	}
}
