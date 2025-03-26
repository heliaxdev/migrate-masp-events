package main

import (
	"flag"
	"fmt"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"

	proto "github.com/heliaxdev/migrate-masp-events/proto/types"
)

type argsPrintBlocks struct {
	SkipEmpty bool
	CometHome string
}

func RegisterCommandPrintBlocks(subCommands map[string]*SubCommand) {
	subCommands["print-blocks"] = &SubCommand{
		Args:        &argsPrintBlocks{},
		Description: "print all data in the blockstore db of cometbft",
		ConfigureFlags: func(iArgs any, flags *flag.FlagSet) {
			args := iArgs.(*argsPrintBlocks)

			flags.StringVar(&args.CometHome, "cometbft-homedir", "", "path to cometbft dir (e.g. .namada/namada.5f5de2dd1b88cba30586420/cometbft)")
			flags.BoolVar(&args.SkipEmpty, "skip-empty", false, "skip printing blocks with no txs")
		},
		Entrypoint: func(iArgs any) error {
			args := iArgs.(*argsPrintBlocks)

			db, err := openBlockStoreDb(args.CometHome)
			if err != nil {
				return err
			}
			defer db.Close()

			return printEndBlocksTxs(db, args.SkipEmpty)
		},
	}
}

func printEndBlocksTxs(db *leveldb.DB, skipEmpty bool) error {
	iter := db.NewIterator(util.BytesPrefix([]byte("abciResponsesKey:")), &opt.ReadOptions{})
	defer iter.Release()

	for h := 1; ; h++ {
		block, err := loadBlock(db, h)
		if err != nil {
			return err
		}
		if block == nil {
			break
		}

		if len(block.Data.Txs) == 0 && skipEmpty {
			continue
		}

		txs := make([]string, 0, 8)

		for i := 0; i < len(block.Data.Txs); i++ {
			tx := &proto.Tx{}
			err = tx.Unmarshal(block.Data.Txs[i])
			if err != nil {
				return fmt.Errorf("unmarshal tx %d of block %d failed: %w", i, h, err)
			}
			txs = append(txs, string(tx.Data))
		}

		fmt.Printf("Header => %#v\n", block.Header)
		fmt.Printf("Txs => %#v\n\n", txs)
	}

	return nil
}
