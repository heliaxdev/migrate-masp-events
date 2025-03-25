package main

import (
	"flag"
	"fmt"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"

	cmtstate "github.com/cometbft/cometbft/proto/tendermint/state"
)

type argsMigrate struct {
	CometHome string
}

func RegisterCommandMigrate(subCommands map[string]*SubCommand) {
	subCommands["migrate"] = &SubCommand{
		Args:        &argsMigrate{},
		Description: "migrate old masp events (<= namada v1.1.4) in the state db of cometbft",
		ConfigureFlags: func(iArgs any, flags *flag.FlagSet) {
			args := iArgs.(*argsMigrate)
			flags.StringVar(&args.CometHome, "cometbft-homedir", "", "path to cometbft dir (e.g. .namada/namada.5f5de2dd1b88cba30586420/cometbft)")
		},
		Entrypoint: func(iArgs any) error {
			args := iArgs.(*argsMigrate)

			db, err := openStateDb(args.CometHome)
			if err != nil {
				return err
			}
			defer db.Close()

			return migrateEvents(db)
		},
	}
}

func migrateEvents(db *leveldb.DB) error {
	iter := db.NewIterator(util.BytesPrefix([]byte("abciResponsesKey:")), &opt.ReadOptions{})
	defer iter.Release()

	updates := make(map[string][]byte)

	for iter.Next() {
		abciResponses := new(cmtstate.ABCIResponses)
		err := abciResponses.Unmarshal(iter.Value())
		if err != nil {
			return fmt.Errorf("failed to unmarshal abci responses from state db: %w", err)
		}
		for i := 0; i < len(abciResponses.EndBlock.Events); i++ {
			abciResponses.EndBlock.Events[i].Type = "asdjaslkdjlkasjdlkjadlj"
		}
		value, err := abciResponses.Marshal()
		if err != nil {
			return fmt.Errorf("failed to marshal abci responses: %w", err)
		}
		updates[string(iter.Key())] = value
	}

	for key, value := range updates {
		err := db.Put([]byte(key), value, new(opt.WriteOptions))
		if err != nil {
			return fmt.Errorf("failed to write migrated abci responses to db: %w", err)
		}
	}

	return nil
}
