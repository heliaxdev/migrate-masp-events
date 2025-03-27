package tx

import (
	stdbin "encoding/binary"
	"fmt"

	"github.com/gagliardetto/binary"
)

type Header struct {
	TxVersion
	ConsensusBranchId
	LockTime     uint32
	ExpiryHeight uint32
}

type Transaction struct {
	Header

	TransparentBundle TransparentBundle
	SaplingBundle     SaplingBundle
}

type TransparentBundle struct {
	Vin  ZcashVector[TransparentTx]
	Vout ZcashVector[TransparentTx]
}

type TransparentTx struct {
	AssetType [32]byte
	Value     uint64
	Address   [20]byte
}

type SaplingBundle struct {
	Spends       ZcashVector[SpendDescription]
	Converts     ZcashVector[ConvertDescription]
	Outputs      ZcashVector[OutputDescription]
	ValueBalance I128Sum
}

func (b *SaplingBundle) UnmarshalWithDecoder(decoder *bin.Decoder) error {
	b.Spends.Data = b.Spends.Data[:0]
	if err := b.Spends.UnmarshalWithDecoder(decoder); err != nil {
		return err
	}

	b.Converts.Data = b.Converts.Data[:0]
	if err := b.Converts.UnmarshalWithDecoder(decoder); err != nil {
		return err
	}

	b.Outputs.Data = b.Outputs.Data[:0]
	if err := b.Outputs.UnmarshalWithDecoder(decoder); err != nil {
		return err
	}

	if len(b.Spends.Data) > 0 || len(b.Converts.Data) > 0 || len(b.Outputs.Data) > 0 {
		if err := b.ValueBalance.UnmarshalWithDecoder(decoder); err != nil {
			return err
		}
	}

	if len(b.Spends.Data) > 0 {
		if err := b.ValueBalance.UnmarshalWithDecoder(decoder); err != nil {
			return err
		}
	}

	return fmt.Errorf("TODO")
}

type SpendDescription struct {
	Cv        [32]byte
	Nullifier [32]byte
	PublicKey [32]byte
}

type ConvertDescription struct {
	Cv [32]byte
}

type OutputDescription struct {
	Cv            [32]byte
	Cmu           [32]byte
	EphemeralKey  [32]byte
	EncCiphertext [580 + 32]byte
	OutCiphertext [80]byte
}

type I128Sum struct{}

type TxVersion struct{}

func (s *TxVersion) UnmarshalWithDecoder(decoder *bin.Decoder) error {
	header, err := decoder.ReadUint32(stdbin.LittleEndian)
	if err != nil {
		return fmt.Errorf("failed to read masp tx version header: %w", err)
	}

	version := header & 0x7FFFFFFF

	versionGroupId, err := decoder.ReadUint32(stdbin.LittleEndian)
	if err != nil {
		return fmt.Errorf("failed to read masp tx version group id: %w", err)
	}

	const (
		MASPV5_TX_VERSION       = 2
		MASPV5_VERSION_GROUP_ID = 0x26A7270A
	)

	if version == MASPV5_TX_VERSION && MASPV5_VERSION_GROUP_ID == 0x26A7270A {
		return nil
	}

	return fmt.Errorf("invalid masp tx version %d and group id %d", version, versionGroupId)
}

type ConsensusBranchId struct{}

func (*ConsensusBranchId) UnmarshalWithDecoder(decoder *bin.Decoder) error {
	branch, err := decoder.ReadUint32(stdbin.LittleEndian)
	if err != nil {
		return fmt.Errorf("failed to consensus branch id: %w", err)
	}

	if branch != 0xe9ff_75a6 {
		return fmt.Errorf("invalid consensus branch id")
	}

	return nil
}
