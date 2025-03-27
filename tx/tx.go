package tx

import (
	"github.com/gagliardetto/binary"
)

type Tx struct {
	Header   Header
	Sections []Section
}

type Header struct {
	ChainId    string
	Expiration string `bin:"optional"`
	Timestamp  string
	Batch      map[TxCommitments]struct{}
	Atomic     bool
	TxType     TxType
}

type Section struct {
	Enum          bin.BorshEnum `borsh_enum:"true"`
	Data          Data
	ExtraData     Code
	Code          Code
	Authorization Authorization
	MaspTx        MaspTx
}

type TxType struct {
	Enum    bin.BorshEnum `borsh_enum:"true"`
	Raw     bin.EmptyVariant
	Wrapper WrapperTx
}

type WrapperTx struct {
	Fee      Fee
	Pk       PublicKey
	GasLimit uint64
}

type PublicKey struct {
	Enum      bin.BorshEnum `borsh_enum:"true"`
	Ed25519   [32]byte
	Secp256k1 [33]byte
}

type Fee struct {
	DenominatedAmount [257]byte
	Token             Address
}

type Address struct {
	Enum        bin.BorshEnum `borsh_enum:"true"`
	Established [20]byte
	Implicit    [20]byte
	Internal    InternalAddress
}

type InternalAddress struct {
	Enum             bin.BorshEnum `borsh_enum:"true"`
	PoS              bin.EmptyVariant
	PosSlashPool     bin.EmptyVariant
	Parameters       bin.EmptyVariant
	Ibc              bin.EmptyVariant
	IbcToken         [20]byte
	Governance       bin.EmptyVariant
	EthBridge        bin.EmptyVariant
	EthBridgePool    bin.EmptyVariant
	Erc20            [20]byte
	Nut              [20]byte
	Multitoken       bin.EmptyVariant
	Pgf              bin.EmptyVariant
	Masp             bin.EmptyVariant
	ReplayProtection bin.EmptyVariant
	TempStorage      bin.EmptyVariant
}

type TxCommitments struct {
	Code [32]byte
	Data [32]byte
	Memo [32]byte
}

type Data struct {
	Salt [8]byte
	Data []byte
}

type Code struct {
	Salt [8]byte
	Code Commitment
	Tag  string `bin:"optional"`
}

type Commitment struct {
	Enum bin.BorshEnum `borsh_enum:"true"`
	Hash [32]byte
	Id   []byte
}

type Authorization struct {
	Targets    [][32]byte
	Signer     Signer
	Signatures map[uint8]Signature
}

type Signer struct {
	Enum    bin.BorshEnum `borsh_enum:"true"`
	Address Address
	PubKeys []PublicKey
}

type Signature struct {
	Enum      bin.BorshEnum `borsh_enum:"true"`
	Ed25519   [64]byte
	Secp256k1 [65]byte
}

type MaspTx struct {
	TxId [32]byte

	Unused [1 + 1 + 4 + 4]byte

	TransparentBundle TransparentBundle `bin:"optional"`
	SaplingBundle     SaplingBundle     `bin:"optional"`
}

type TransparentBundle struct {
	Vin  []TransparentTx
	Vout []TransparentTx
}

type TransparentTx struct {
	AssetType [32]byte
	Value     uint64
	Address   [20]byte
}

type SaplingBundle struct {
	Spends       []SpendDescription
	Converts     []ConvertDescription
	Outputs      []OutputDescription
	ValueBalance I128Sum
}

type SpendDescription struct{}

type ConvertDescription struct{}

type OutputDescription struct{}

type I128Sum struct{}
