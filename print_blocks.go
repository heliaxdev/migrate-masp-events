package main

import (
	"flag"
	"fmt"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
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

		fmt.Printf("%#v\n\n", block)
	}

	return nil
}
