package main

import (
	"flag"
	"fmt"
	"slices"
	"sync"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"

	"github.com/cometbft/cometbft/abci/types"
	cmtstate "github.com/cometbft/cometbft/proto/tendermint/state"
)

type argsMigrate struct {
	CometHome   string
	MaspIndexer string
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
			flags.StringVar(
				&args.MaspIndexer,
				"masp-indexer",
				"",
				"url of masp indexer dir (e.g. https://bing.bong/api/v1)",
			)
		},
		Entrypoint: func(iArgs any) error {
			args := iArgs.(*argsMigrate)

			db, err := openStateDb(args.CometHome)
			if err != nil {
				return err
			}
			defer db.Close()

			return migrateEvents(db, args.MaspIndexer)
		},
	}
}

func migrateEvents(db *leveldb.DB, maspIndexerUrl string) error {
	if maspIndexerUrl == "" {
		return fmt.Errorf("masp indexer url was not set")
	}

	maspIndexerClient := NewMaspIndexerClient(maspIndexerUrl)

	iter := db.NewIterator(util.BytesPrefix([]byte("abciResponsesKey:")), &opt.ReadOptions{})
	defer iter.Release()

	wg := &sync.WaitGroup{}
	sem := make(chan struct{}, MaxConcurrentRequests)
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

			heightStr := extractHeight(iter.Key())
			height, err := parseHeight(heightStr)
			if err != nil {
				errs <- err
				return
			}

			maspTxs, err := maspIndexerClient.BlockHeight(height)
			if err != nil {
				errs <- err
				return
			}

			dirty = dirty || len(maspTxs) > 0

			// TODO:
			// - need to open leveldb db that contains block responses (with tx data)
			// - check what kind of tx there is at the given block index and height
			// 		- if it is an ibc shielding, compute the data section hash
			// 		- if it is a regular transfer tx, compute the masp tx id
			// - will probably need to call rust code :((((((
			for i := 0; i < len(maspTxs); i++ {
				for j := 0; j < len(maspTxs[i].Batch); j++ {
					abciResponses.EndBlock.Events = append(
						abciResponses.EndBlock.Events,
						types.Event{
							Type: "masp/transfer",
							Attributes: []types.EventAttribute{
								{
									Key:   "height",
									Value: heightStr,
									Index: true,
								},
								{
									Key: "indexed-tx",
									Value: fmt.Sprintf(
										`{"block_height":%s,"block_index":%d,"batch_index":%d}`,
										heightStr,
										maspTxs[i].BlockIndex,
										maspTxs[i].Batch[j].MaspTxIndex,
									),
									Index: true,
								},
								{
									Key: "section",
									Value: fmt.Sprintf(
										`{"MaspSection":%s}`, /* TODO: could also be IBC */
										"",                   /* TODO */
									),
									Index: true,
								},
								{
									Key:   "event-level",
									Value: "tx",
									Index: true,
								},
							},
						},
					)
				}
			}

			if !dirty {
				// NB: no changes need to be made to the db
				return
			}

			value, err = abciResponses.Marshal()
			if err != nil {
				errs <- fmt.Errorf("failed to marshal abci responses: %w", err)
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
