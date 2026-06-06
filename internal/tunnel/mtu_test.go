package tunnel

import "testing"

func TestFramingOverhead(t *testing.T) {
	cases := map[int]int{
		0:     1, // varint(0) = 1 byte
		127:   1, // fits in one varint byte
		128:   2, // needs two
		256:   2,
		16383: 2,
		16384: 3,
	}
	for padMax, want := range cases {
		if got := FramingOverhead(padMax); got != want {
			t.Errorf("FramingOverhead(%d) = %d, want %d", padMax, got, want)
		}
	}
}

func TestInnerMTU(t *testing.T) {
	// No padding: inner MTU = maxDatagram - 1 (the pad_len varint), capped.
	if got := InnerMTU(1350, 0, 0); got != 1349 {
		t.Errorf("InnerMTU(1350,0,0) = %d, want 1349", got)
	}
	// With padding, both the varint and the padding bytes are subtracted.
	if got := InnerMTU(1350, 256, 0); got != 1350-2-256 {
		t.Errorf("InnerMTU(1350,256,0) = %d, want %d", got, 1350-2-256)
	}
	// Upper bound clamps a generous datagram down to the configured MTU.
	if got := InnerMTU(9000, 0, 1280); got != 1280 {
		t.Errorf("InnerMTU(9000,0,1280) = %d, want 1280", got)
	}
	// Floor clamps a tiny datagram up to the minimum.
	if got := InnerMTU(200, 0, 0); got != MinInnerMTU {
		t.Errorf("InnerMTU(200,0,0) = %d, want %d", got, MinInnerMTU)
	}
}

// TestInnerMTUGuarantee is the core invariant of the MTU gate (§3.4): a packet
// as large as the inner MTU, once framed with the maximum padding, must still
// fit within the max datagram. This is what stops GSO-split packets and normal
// traffic from silently hitting the oversize-drop path.
func TestInnerMTUGuarantee(t *testing.T) {
	for _, maxDg := range []int{700, 1200, 1252, 1350, 1452} {
		for _, padMax := range []int{0, 16, 256, 1000} {
			mtu := InnerMTU(maxDg, padMax, 0)
			framed := mtu + FramingOverhead(padMax) + padMax
			if mtu > MinInnerMTU && framed > maxDg {
				t.Errorf("framed packet %d exceeds max datagram %d (mtu=%d padMax=%d)",
					framed, maxDg, mtu, padMax)
			}
		}
	}
}
