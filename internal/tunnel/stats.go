package tunnel

import (
	"fmt"
	"sync/atomic"
)

// Stats holds per-tunnel data-plane counters. All fields are updated with
// atomic operations so they can be read from a signal handler while the pumps
// run.
//
// The point of splitting the drop counters is to tell apart "the inner TCP is
// retransmitting because the path lost packets" (normal) from "we are dropping
// datagrams ourselves" (a bug or an under-sized buffer). Compare the sender's
// TxSent with the receiver's RxRecv: on a lossless link they should match
// closely; a large gap means datagrams died between SendDatagram and
// ReceiveDatagram (kernel UDP buffer or quic-go's 128-deep receive queue).
type Stats struct {
	// Outbound: TUN -> datagram.
	TxTUN          atomic.Uint64 // IP packets read from the TUN
	TxSent         atomic.Uint64 // datagrams handed to SendDatagram successfully
	TxDropOversize atomic.Uint64 // packets dropped because they exceed the max datagram size
	TxDropErr      atomic.Uint64 // datagrams dropped due to a (fatal) send error

	// Inbound: datagram -> TUN.
	RxRecv          atomic.Uint64 // datagrams returned by ReceiveDatagram
	RxWritten       atomic.Uint64 // IP packets written to the TUN
	RxDropMalformed atomic.Uint64 // datagrams that failed to decode
	RxDropBacklog   atomic.Uint64 // datagrams dropped because the TUN-write backlog was full
	RxDropTun       atomic.Uint64 // packets dropped on TUN write error
}

// String renders a single-line summary suitable for logs and the test harness
// (which greps for the "STATS" prefix).
func (s *Stats) String() string {
	return fmt.Sprintf(
		"STATS tx_tun=%d tx_sent=%d tx_drop_oversize=%d tx_drop_err=%d "+
			"rx_recv=%d rx_written=%d rx_drop_malformed=%d rx_drop_backlog=%d rx_drop_tun=%d",
		s.TxTUN.Load(), s.TxSent.Load(), s.TxDropOversize.Load(), s.TxDropErr.Load(),
		s.RxRecv.Load(), s.RxWritten.Load(), s.RxDropMalformed.Load(),
		s.RxDropBacklog.Load(), s.RxDropTun.Load(),
	)
}
