package tunnel

import "encoding/binary"

// MinInnerMTU is the floor we will clamp the inner MTU to. 576 is the classic
// IPv4 minimum reassembly buffer; below this almost nothing works.
const MinInnerMTU = 576

// FramingOverhead returns the number of header bytes EncodeDatagram prepends in
// the worst case: the varint encoding the pad length. The pad length never
// exceeds padMax, so the varint for padMax bounds the overhead.
func FramingOverhead(padMax int) int {
	if padMax < 0 {
		padMax = 0
	}
	var b [binary.MaxVarintLen64]byte
	return binary.PutUvarint(b[:], uint64(padMax))
}

// InnerMTU computes the largest inner-IP MTU that still guarantees every framed
// datagram (packet + framing + up to padMax padding) fits within maxDatagram —
// the runtime-measured max QUIC DATAGRAM payload (§3.4 MTU gate). It is clamped
// to MinInnerMTU and to the caller-supplied upper bound.
//
// Because the largest packet the kernel hands us equals the TUN MTU, setting
// the TUN MTU to this value means a normal packet never overflows the datagram:
//
//	maxPacket(=InnerMTU) + FramingOverhead(padMax) + padMax <= maxDatagram
func InnerMTU(maxDatagram, padMax, upperBound int) int {
	mtu := maxDatagram - FramingOverhead(padMax) - padMax
	if upperBound > 0 && mtu > upperBound {
		mtu = upperBound
	}
	if mtu < MinInnerMTU {
		mtu = MinInnerMTU
	}
	return mtu
}
