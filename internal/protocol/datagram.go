// Package protocol defines the SiegaNet wire format for the data-plane and,
// later, the control-plane. The data-plane carries exactly one IP packet per
// QUIC DATAGRAM (RFC 9221), framed so that random padding can be appended to
// blur packet-size signatures.
//
// Datagram payload layout (§3.4 of the build brief):
//
//	[varint pad_len][raw_ip_packet][pad_len bytes of CSPRNG garbage]
//
// The decoder reads pad_len from the front; the IP packet is everything
// between the varint and the trailing pad_len bytes. The total datagram length
// is known from the transport, so no explicit packet-length field is needed.
package protocol

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	mathrand "math/rand/v2"
)

// ErrShortDatagram is returned when a received datagram is too short to be a
// valid framed packet (truncated varint or pad_len exceeding the payload).
var ErrShortDatagram = errors.New("protocol: datagram too short or malformed")

// PadRange describes the inclusive range of random padding bytes appended to
// each outgoing datagram. Min == Max == 0 disables padding (Phase 0 default).
type PadRange struct {
	Min int
	Max int
}

// pick returns a random pad length within the range using a fast, non-crypto
// PRNG (the padding content is CSPRNG, but its *length* need not be).
func (p PadRange) pick() int {
	if p.Max <= 0 || p.Max <= p.Min {
		if p.Min > 0 {
			return p.Min
		}
		return 0
	}
	return p.Min + mathrand.IntN(p.Max-p.Min+1)
}

// EncodeDatagram frames a single IP packet into a datagram payload, appending a
// random amount of CSPRNG padding chosen from r. The result is written into dst
// (which may be nil); the possibly-reallocated slice is returned. The caller
// may reuse dst across calls to avoid allocations.
func EncodeDatagram(dst []byte, ipPacket []byte, r PadRange) ([]byte, error) {
	padLen := r.pick()

	// Reserve room: up to 10 bytes of varint (varint of an int fits in <=10) +
	// packet + padding. In practice pad_len is small so the varint is 1-2 bytes.
	need := binary.MaxVarintLen64 + len(ipPacket) + padLen
	if cap(dst) < need {
		dst = make([]byte, 0, need)
	}
	dst = dst[:0]

	var hdr [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(hdr[:], uint64(padLen))
	dst = append(dst, hdr[:n]...)
	dst = append(dst, ipPacket...)

	if padLen > 0 {
		start := len(dst)
		dst = dst[:start+padLen]
		if _, err := rand.Read(dst[start:]); err != nil {
			return nil, err
		}
	}
	return dst, nil
}

// DecodeDatagram extracts the IP packet from a received datagram payload. The
// returned slice aliases dg (no copy); the caller must copy it before the next
// read if it needs to retain the data.
func DecodeDatagram(dg []byte) ([]byte, error) {
	padLen64, n := binary.Uvarint(dg)
	if n <= 0 {
		return nil, ErrShortDatagram
	}
	rest := dg[n:]
	padLen := int(padLen64)
	if padLen < 0 || padLen > len(rest) {
		return nil, ErrShortDatagram
	}
	return rest[:len(rest)-padLen], nil
}
