package main

import (
	"flag"
	"fmt"
	"path/filepath"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"

	cmtstate "github.com/cometbft/cometbft/proto/tendermint/state"
)

type argsPrint struct {
	CometHome string
}

func RegisterCommandPrint(subCommands map[string]*SubCommand) {
	subCommands["print"] = &SubCommand{
		Args:        &argsPrint{},
		Description: "print all end blocks instances in the state db of cometbft",
		ConfigureFlags: func(iArgs any, flags *flag.FlagSet) {
			args := iArgs.(*argsPrint)
			flags.StringVar(&args.CometHome, "cometbft-homedir", "", "path to cometbft dir (e.g. .namada/namada.5f5de2dd1b88cba30586420/cometbft)")
		},
		Entrypoint: func(iArgs any) error {
			args := iArgs.(*argsPrint)

			if args.CometHome == "" {
				return fmt.Errorf("no cometbft home dir provided as arg")
			}

			dbPath := filepath.Join(args.CometHome, "data", "state.db")

			db, err := leveldb.OpenFile(dbPath, nil)
			if err != nil {
				return fmt.Errorf("failed to open db in %s: %w", dbPath, err)
			}
			defer db.Close()

			return printEndBlocks(db)
		},
	}
}

func printEndBlocks(db *leveldb.DB) error {
	iter := db.NewIterator(util.BytesPrefix([]byte("abciResponsesKey:")), &opt.ReadOptions{})
	defer iter.Release()

	for iter.Next() {
		abciResponses := new(cmtstate.ABCIResponses)
		err := abciResponses.Unmarshal(iter.Value())
		if err != nil {
			return fmt.Errorf("failed to unmarshal abci responses from state db: %w", err)
		}
		fmt.Printf("Key = %q\n", string(iter.Key()))
		fmt.Printf("Value = %#v\n", abciResponses.EndBlock)
	}

	return nil
}
