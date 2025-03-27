package tx

import (
	stdbin "encoding/binary"
	"fmt"

	"github.com/gagliardetto/binary"
)

type ZcashCompactSize struct {
	Size uint64
}

func (s *ZcashCompactSize) UnmarshalWithDecoder(decoder *bin.Decoder) error {
	tag, err := decoder.ReadUint8()
	if err != nil {
		return err
	}

	switch {
	case tag < 253:
		s.Size = uint64(tag)
	case tag == 253:
		n, err := decoder.ReadUint16(stdbin.LittleEndian)
		if err != nil {
			return err
		}
		if n < 253 {
			return fmt.Errorf("got tag 253 but compact size is %d", n)
		}
		s.Size = uint64(n)
	case tag == 254:
		n, err := decoder.ReadUint32(stdbin.LittleEndian)
		if err != nil {
			return err
		}
		if n < 0x10000 {
			return fmt.Errorf("got tag 254 but compact size is %d", n)
		}
		s.Size = uint64(n)
	default: // 255
		n, err := decoder.ReadUint64(stdbin.LittleEndian)
		if err != nil {
			return err
		}
		if n < 0x100000000 {
			return fmt.Errorf("got tag >254 but compact size is %d", n)
		}
		s.Size = uint64(n)
	}

	return nil
}
