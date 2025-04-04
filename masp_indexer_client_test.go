package main

import "testing"

func TestMaspTxSorting(t *testing.T) {
	testResponse := serverResponse{
		Txs: []Transaction{
			{
				BlockIndex: 0,
				Batch: []TransactionSlot{
					{MaspTxIndex: 1},
				},
			},
			{
				BlockIndex: 3,
				Batch: []TransactionSlot{
					{MaspTxIndex: 0},
				},
			},
		},
	}

	testResponse.sortMaspTxs()

	if testResponse.Txs[0].BlockIndex != 3 {
		t.Fatalf("invalid masp tx sort order: %#v\n", testResponse)
	}
}
