package tx

import (
	stdbin "encoding/binary"
	"fmt"

	"github.com/gagliardetto/binary"
)

type ZcashVector[T bin.BinaryUnmarshaler] struct {
	Data []T
}

func (v *ZcashVector[T]) UnmarshalWithDecoder(decoder *bin.Decoder) error {
	var c ZcashCompactSize

	err := c.UnmarshalWithDecoder(decoder)
	if err != nil {
		return fmt.Errorf("failed to decode vec size: %w", err)
	}

	v.Data = v.Data[:0]

	for i := uint64(0); i < c.Size; i++ {
		var decoded T
		err = decoded.UnmarshalWithDecoder(decoder)
		if err != nil {
			return fmt.Errorf("failed to decode vec elem: %w", err)
		}
		v.Data = append(v.Data, decoded)
	}

	return nil
}

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
