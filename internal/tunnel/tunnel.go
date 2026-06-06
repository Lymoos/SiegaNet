// Package tunnel pumps IP packets between a TUN device and a QUIC connection's
// DATAGRAM channel. One IP packet maps to exactly one QUIC DATAGRAM (§0.2).
//
// Two goroutines run per tunnel:
//
//	outbound: TUN.Read  -> protocol.EncodeDatagram -> conn.SendDatagram
//	inbound:  conn.ReceiveDatagram -> protocol.DecodeDatagram -> TUN.Write
package tunnel

import (
	"context"
	"errors"
	"log"

	"github.com/quic-go/quic-go"
	"golang.zx2c4.com/wireguard/tun"

	"github.com/lymoos/sieganet/internal/protocol"
	"github.com/lymoos/sieganet/internal/tundev"
)

const maxPacket = 65535

// Run drives the data-plane until the connection or context ends. It returns
// the first fatal error from either direction.
func Run(ctx context.Context, dev tun.Device, conn *quic.Conn, pad protocol.PadRange) error {
	errc := make(chan error, 2)
	go func() { errc <- pumpOutbound(dev, conn, pad) }()
	go func() { errc <- pumpInbound(ctx, dev, conn) }()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errc:
		return err
	}
}

// pumpOutbound reads packets from the TUN and sends each as a DATAGRAM.
func pumpOutbound(dev tun.Device, conn *quic.Conn, pad protocol.PadRange) error {
	batch := dev.BatchSize()
	bufs := make([][]byte, batch)
	sizes := make([]int, batch)
	for i := range bufs {
		bufs[i] = make([]byte, tundev.Offset+maxPacket)
	}
	var sendBuf []byte

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
			sendBuf, err = protocol.EncodeDatagram(sendBuf, pkt, pad)
			if err != nil {
				return err
			}
			if err := conn.SendDatagram(sendBuf); err != nil {
				// A datagram that does not fit the negotiated max size is
				// dropped (§3.4). Other errors mean the connection is gone.
				var tooLarge *quic.DatagramTooLargeError
				if errors.As(err, &tooLarge) {
					log.Printf("tunnel: dropping oversized packet (%d bytes, max %d)", len(pkt), tooLarge.MaxDatagramPayloadSize)
					continue
				}
				return err
			}
		}
	}
}

// pumpInbound receives DATAGRAMs and writes the carried IP packet to the TUN.
func pumpInbound(ctx context.Context, dev tun.Device, conn *quic.Conn) error {
	full := make([]byte, tundev.Offset+maxPacket)
	wb := make([][]byte, 1)

	for {
		dg, err := conn.ReceiveDatagram(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}
		pkt, err := protocol.DecodeDatagram(dg)
		if err != nil || len(pkt) == 0 {
			continue // malformed/empty datagram; ignore
		}
		copy(full[tundev.Offset:], pkt)
		wb[0] = full[: tundev.Offset+len(pkt)]
		if _, err := dev.Write(wb, tundev.Offset); err != nil {
			log.Printf("tunnel: tun write: %v", err)
		}
	}
}
