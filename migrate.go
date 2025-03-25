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
				"url of masp indexer (e.g. https://bing.bong/api/v1)",
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
		select {
		case mainErr = <-errs:
			break
		default:
		}

		wg.Add(1)
		sem <- struct{}{}

		go func(key, value []byte) {
			defer func() {
				<-sem
				wg.Done()
			}()

			containsMaspDataRefs := false

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
							containsMaspDataRefs = true
						}
					}
				case "masp/transfer", "masp/fee-payment":
					// NB: this db has already been migrated
					errs <- nil
					return
				}
			}

			if !containsMaspDataRefs {
				return
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
								// TODO:
								// - need to open leveldb db that contains block responses (with tx data)
								// - query the namada tx at the given block height and block index
								// - use protobuf to decode the tx bytes. then, borsh decode the tx
								// 	  - grab the `MaspTxId` from the data fetched from the masp indexer
								//    - look for a matching `Section::MaspTx`. if it is not present
								//    in the tx, then it is an ibc shielding (`{"IbcData":"AABBCC010203..."}`);
								//    otherwise, it is an internal masp tx (`{"MaspSection":[1,2,3,...]}`).
								//       - still need to locate the data section containing the ibc shielding,
								//       because its hash will be stored in the event
								//    - look at c# code for inspiration
								//       - https://github.com/heliaxdev/bitcoin-suisse-tx/blob/main/Tx.cs
								//
								//   ...
								//
								// - alternatively, we could tag everything as "MaspSection", and inject a new
								//   masp tx section into the namada txs...
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

	// NB: one last attempt at getting errors
	select {
	case freshErr := <-errs:
		// NB: avoid non-errs overriding actual errs
		if freshErr != nil {
			mainErr = freshErr
		}
	default:
	}

	return mainErr
}

func swapRemove[T any](slice []T, index int) []T {
	slice[index] = slice[len(slice)-1]
	return slice[:len(slice)-1]
}
