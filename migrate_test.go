package main

import (
	"testing"

	"github.com/cometbft/cometbft/abci/types"
	"github.com/heliaxdev/migrate-masp-events/namada"
)

func TestMaspEventIndexedTx1(t *testing.T) {
	events, err := emitMaspEvents(
		1055118,
		nil,
		[]indexedMaspSection{
			{
				section: namada.MaspTxSection{
					Ibc: true,
					Hash: [32]byte{
						102, 49, 169, 245, 212, 8, 154, 9, 166,
						134, 171, 224, 188, 96, 184, 179, 84, 250,
						20, 51, 138, 60, 200, 126, 104, 173, 28, 67,
						195, 110, 88, 124,
					},
				},
			},
			{
				section: namada.MaspTxSection{
					Ibc: false,
					Hash: [32]byte{
						163, 68, 193, 252, 127, 61, 25, 118, 108, 205, 16, 32, 254,
						198, 95, 253, 149, 133, 118, 112, 241, 75, 120, 102, 67, 43,
						122, 118, 223, 72, 173, 0,
					},
				},
			},
		},
		[]indexedMaspSection{
			{
				section: namada.MaspTxSection{
					Ibc: false,
					Hash: [32]byte{
						163, 68, 193, 252, 127, 61, 25, 118, 108, 205, 16, 32, 254,
						198, 95, 253, 149, 133, 118, 112, 241, 75, 120, 102, 67, 43,
						122, 118, 223, 72, 173, 0,
					},
				},
			},
			{
				section: namada.MaspTxSection{
					Ibc: true,
					Hash: [32]byte{
						102, 49, 169, 245, 212, 8, 154, 9, 166,
						134, 171, 224, 188, 96, 184, 179, 84, 250,
						20, 51, 138, 60, 200, 126, 104, 173, 28, 67,
						195, 110, 88, 124,
					},
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("failed to emit masp events: %s", err)
	}

	hasExpectedFeePayments(t, events, []int{0})
}

func TestMaspEventIndexedTx2(t *testing.T) {
	events, err := emitMaspEvents(
		1234,
		nil,
		[]indexedMaspSection{
			{section: namada.MaspTxSection{false, [32]byte{1}}},
			{section: namada.MaspTxSection{false, [32]byte{2}}},
			{section: namada.MaspTxSection{false, [32]byte{3}}},
			{section: namada.MaspTxSection{true, [32]byte{'A'}}},
			{section: namada.MaspTxSection{false, [32]byte{4}}},
		},
		[]indexedMaspSection{
			{section: namada.MaspTxSection{false, [32]byte{1}}},
			{section: namada.MaspTxSection{false, [32]byte{3}}},
			{section: namada.MaspTxSection{false, [32]byte{4}}},
			{section: namada.MaspTxSection{false, [32]byte{2}}},
			{section: namada.MaspTxSection{true, [32]byte{'A'}}},
		},
	)
	if err != nil {
		t.Fatalf("failed to emit masp events: %s", err)
	}

	hasExpectedFeePayments(t, events, []int{0, 1, 2})
}

func hasExpectedFeePayments(
	t *testing.T,
	events []types.Event,
	expectedFeePaymentIndices []int,
) {
	for _, i := range expectedFeePaymentIndices {
		if events[i].Type != "masp/fee-payment" {
			t.Fatalf("masp event is not fee payment: %#v", events[i])
		}
	}
}
