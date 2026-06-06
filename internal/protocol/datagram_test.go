package protocol

import (
	"bytes"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		pkt  []byte
		pad  PadRange
	}{
		{"empty-pad small packet", []byte{0x45, 0x00, 0x00, 0x14}, PadRange{}},
		{"fixed pad", bytes.Repeat([]byte{0xab}, 100), PadRange{Min: 16, Max: 16}},
		{"range pad", bytes.Repeat([]byte{0xcd}, 1280), PadRange{Min: 0, Max: 256}},
		{"single byte", []byte{0xff}, PadRange{Min: 1, Max: 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dg, err := EncodeDatagram(nil, tc.pkt, tc.pad)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			got, err := DecodeDatagram(dg)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if !bytes.Equal(got, tc.pkt) {
				t.Fatalf("roundtrip mismatch:\n got %x\nwant %x", got, tc.pkt)
			}
			// Encoded length must be at least packet + min pad.
			if len(dg) < len(tc.pkt)+tc.pad.Min {
				t.Fatalf("encoded len %d < packet %d + minpad %d", len(dg), len(tc.pkt), tc.pad.Min)
			}
		})
	}
}

func TestEncodeReusesBuffer(t *testing.T) {
	pkt := bytes.Repeat([]byte{0x01}, 50)
	buf, err := EncodeDatagram(nil, pkt, PadRange{})
	if err != nil {
		t.Fatal(err)
	}
	cap0 := cap(buf)
	// Re-encode a smaller packet into the same buffer; should not reallocate.
	buf2, err := EncodeDatagram(buf, pkt[:10], PadRange{})
	if err != nil {
		t.Fatal(err)
	}
	if cap(buf2) != cap0 {
		t.Fatalf("buffer reallocated: cap %d -> %d", cap0, cap(buf2))
	}
	got, err := DecodeDatagram(buf2)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, pkt[:10]) {
		t.Fatalf("got %x want %x", got, pkt[:10])
	}
}

func TestDecodeMalformed(t *testing.T) {
	// pad_len varint claims more padding than the payload holds.
	bad := []byte{0x7f, 0x01, 0x02} // pad_len=127 but only 2 bytes follow
	if _, err := DecodeDatagram(bad); err == nil {
		t.Fatal("expected error for oversized pad_len")
	}
	if _, err := DecodeDatagram(nil); err == nil {
		t.Fatal("expected error for empty datagram")
	}
}
