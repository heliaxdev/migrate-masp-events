package tx

import (
	"testing"

	"github.com/gagliardetto/binary"
)

func TestZcashCompactSize(t *testing.T) {
	evalSucc := func(expected uint64, data []byte) {
		var c ZcashCompactSize

		d := bin.NewBorshDecoder(data)

		if err := d.Decode(&c); err != nil {
			t.Fatalf("failed to decode %v: %s", data, err)
		}

		if c.Size != expected {
			t.Fatalf("failed to decode %v: expected %d, got %d", data, expected, c.Size)
		}
	}
	evalFail := func(data []byte) {
		var c ZcashCompactSize

		d := bin.NewBorshDecoder(data)

		if err := d.Decode(&c); err == nil {
			t.Fatalf("decode succeeded: %v", data)
		}
	}

	evalSucc(0, []byte{0})
	evalSucc(1, []byte{1})
	evalSucc(252, []byte{252})
	evalSucc(253, []byte{253, 253, 0})
	evalSucc(254, []byte{253, 254, 0})
	evalSucc(255, []byte{253, 255, 0})
	evalSucc(256, []byte{253, 0, 1})
	evalSucc(256, []byte{253, 0, 1})
	evalSucc(65535, []byte{253, 255, 255})
	evalSucc(65536, []byte{254, 0, 0, 1, 0})
	evalSucc(65537, []byte{254, 1, 0, 1, 0})
	evalSucc(33554432, []byte{254, 0, 0, 0, 2})
	evalSucc(0x100000022, []byte{255, 34, 0, 0, 0, 1, 0, 0, 0})

	evalFail([]byte{253})
	evalFail([]byte{254})
	evalFail([]byte{255})
	evalFail([]byte{253, 0})
	evalFail([]byte{254, 1, 0, 0, 0})
	evalFail([]byte{255, 0, 0, 0, 0, 0, 0, 0})
	evalFail([]byte{255, 0, 0, 0, 0, 0, 0, 0, 0})
}
