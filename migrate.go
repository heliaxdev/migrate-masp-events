package main

import (
	"flag"
	"fmt"
	"slices"
	"sync"

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

			flags.StringVar(
				&args.CometHome,
				"cometbft-homedir",
				"",
				"path to cometbft dir (e.g. .namada/namada.5f5de2dd1b88cba30586420/cometbft)",
			)
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

	wg := &sync.WaitGroup{}
	sem := make(chan struct{}, 128)
	errs := make(chan error, 1)

	var mainErr error

	for iter.Next() {
		wg.Add(1)
		sem <- struct{}{}

		select {
		case mainErr = <-errs:
			break
		default:
		}

		go func(key, value []byte) {
			defer func() {
				<-sem
				wg.Done()
			}()

			dirty := false

			abciResponses := new(cmtstate.ABCIResponses)
			err := abciResponses.Unmarshal(value)

			if err != nil {
				errs <- fmt.Errorf("failed to unmarshal abci responses from state db: %w", err)
				return
			}

			for i := 0; i < len(abciResponses.EndBlock.Events); i++ {
				switch abciResponses.EndBlock.Events[i].Type {
				case "tx/applied":
					for e := 0; e < len(abciResponses.EndBlock.Events[i].Attributes); e++ {
						if abciResponses.EndBlock.Events[i].Attributes[e].Key == "masp_data_refs" {
							abciResponses.EndBlock.Events[i].Attributes = swapRemove(
								abciResponses.EndBlock.Events[i].Attributes,
								e,
							)
							dirty = true
						}
					}
				case "masp/transfer", "masp/fee-payment":
					// NB: this db has already been migrated
					errs <- nil
					return
				}
			}

			value, err = abciResponses.Marshal()
			if err != nil {
				errs <- fmt.Errorf("failed to marshal abci responses: %w", err)
				return
			}

			// TODO: fetch new masp events and add them here

			if !dirty {
				// NB: no changes were made to the db
				return
			}

			err = db.Put(key, value, new(opt.WriteOptions))
			if err != nil {
				errs <- fmt.Errorf("failed to write migrated abci responses to db: %w", err)
			}
		}(slices.Clone(iter.Key()), slices.Clone(iter.Value()))
	}

	wg.Wait()

	return mainErr
}

func swapRemove[T any](slice []T, index int) []T {
	slice[index] = slice[len(slice)-1]
	return slice[:len(slice)-1]
}
