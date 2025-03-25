package main

import (
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"

	cmtstate "github.com/cometbft/cometbft/proto/tendermint/state"
)

// TODO:
//
// - include backup/restore functionality to dump old block events
//   and load them again if something goes wrong
// - hardcode phase3 start height, to only process the events in
//   those blocks and speed up the migration
// - check if end block events already contain `masp/transfer` or
//   `masp/fee-payment` events for this program to be indempotent

func main() {
	db, err := leveldb.OpenFile("/Users/sugo/Code/heliax/namada/.namada/validator-0/local.49606e92a35a899e7e2559ef/cometbft/data/state.db", nil)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	migrateEvents(db)
	printEndBlocks(db)
}

func migrateEvents(db *leveldb.DB) {
	iter := db.NewIterator(util.BytesPrefix([]byte("abciResponsesKey:")), &opt.ReadOptions{})
	defer iter.Release()

	updates := make(map[string][]byte)

	for iter.Next() {
		abciResponses := new(cmtstate.ABCIResponses)
		err := abciResponses.Unmarshal(iter.Value())
		if err != nil {
			panic(err)
		}
		for i := 0; i < len(abciResponses.EndBlock.Events); i++ {
			abciResponses.EndBlock.Events[i].Type = "asdjaslkdjlkasjdlkjadlj"
		}
		value, err := abciResponses.Marshal()
		if err != nil {
			panic(err)
		}
		updates[string(iter.Key())] = value
	}

	for key, value := range updates {
		err := db.Put([]byte(key), value, new(opt.WriteOptions))
		if err != nil {
			panic(err)
		}
	}
}
