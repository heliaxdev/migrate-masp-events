package namada

/*
#cgo LDFLAGS: -Lparse/target/release -lparse

#include <stddef.h>
#include <stdint.h>

extern int8_t compute_masp_tx_id(
	void *result_ptr,
	void *borsh_data_ptr,
	size_t borsh_data_len
);

extern int8_t locate_masp_tx_ids_in_nam_tx(
	void *result_ptr,
	void *nam_tx_borsh_data_ptr,
	size_t nam_tx_borsh_data_len
);
*/
import "C"

import (
	"fmt"
	"runtime/cgo"
	"unsafe"
)

type MaspTxSection struct {
	Ibc  bool
	Hash [32]byte
}

//export append_masp_tx
func append_masp_tx(mapHandle, maspTxId, sectionHash unsafe.Pointer, isIbc C.int8_t) {
	sections := cgo.Handle(mapHandle).Value().(map[[32]byte]MaspTxSection)

	sections[*(*[32]byte)(maspTxId)] = MaspTxSection{
		Ibc:  isIbc != 0,
		Hash: *(*[32]byte)(sectionHash),
	}
}

func ComputeMaspTxId(maspTxBorshData []byte) (maspTxId [32]byte, err error) {
	code := C.compute_masp_tx_id(
		unsafe.Pointer(&maspTxId),
		unsafe.Pointer(&maspTxBorshData[0]),
		C.size_t(len(maspTxBorshData)),
	)

	switch code {
	case -1:
		err = fmt.Errorf("failed to deserialize borsh masp tx data from rust")
	default:
		err = fmt.Errorf("null pointers passed to rust")
	}

	return
}

func LocateMaspTxIdsInNamTx(namadaTxBorshData []byte) (map[[32]byte]MaspTxSection, error) {
	maspTxIds := make(map[[32]byte]MaspTxSection)

	h := cgo.NewHandle(maspTxIds)
	defer h.Delete()

	code := C.locate_masp_tx_ids_in_nam_tx(
		unsafe.Pointer(h),
		unsafe.Pointer(&namadaTxBorshData[0]),
		C.size_t(len(namadaTxBorshData)),
	)

	switch code {
	case 0:
		return maspTxIds, nil
	case -1:
		return nil, fmt.Errorf("failed to deserialize borsh namada tx data from rust")
	default:
		return nil, fmt.Errorf("null pointers passed to rust")
	}
}
