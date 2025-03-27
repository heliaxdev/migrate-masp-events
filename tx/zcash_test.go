package tx

import (
	"testing"

	"github.com/gagliardetto/binary"
)

func TestZcashCompactSize(t *testing.T) {
	eval := func(expected uint64, data []byte) {
		var c ZcashCompactSize

		d := bin.NewBorshDecoder(data)

		if err := d.Decode(&c); err != nil {
			t.Fatalf("failed to decode %v: %s", data, err)
		}

		if c.Size != expected {
			t.Fatalf("failed to decode %v: expected %d, got %d", data, expected, c.Size)
		}
	}

	eval(0, []byte{0})
	eval(1, []byte{1})
	eval(252, []byte{252})
	eval(253, []byte{253, 253, 0})
	eval(254, []byte{253, 254, 0})
	eval(255, []byte{253, 255, 0})
	eval(256, []byte{253, 0, 1})
	eval(256, []byte{253, 0, 1})
	eval(65535, []byte{253, 255, 255})
	eval(65536, []byte{254, 0, 0, 1, 0})
	eval(65537, []byte{254, 1, 0, 1, 0})
	eval(33554432, []byte{254, 0, 0, 0, 2})
}
