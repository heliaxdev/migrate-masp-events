package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"strconv"
	"sync"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"

	"github.com/cometbft/cometbft/abci/types"
	cmtstate "github.com/cometbft/cometbft/proto/tendermint/state"

	"github.com/heliaxdev/migrate-masp-events/namada"
	namproto "github.com/heliaxdev/migrate-masp-events/proto/types"
)

type argsMigrate struct {
	CometHome              string
	MaspIndexer            string
	StartHeight, EndHeight int
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
			flags.IntVar(
				&args.StartHeight,
				"start",
				1,
				"block height to start migrations",
			)
			flags.IntVar(
				&args.EndHeight,
				"end",
				0,
				"block height to stop migrations (default is last committed height)",
			)
		},
		Entrypoint: func(iArgs any) error {
			args := iArgs.(*argsMigrate)

			stateDb, err := openStateDb(args.CometHome)
			if err != nil {
				return err
			}
			defer stateDb.Close()

			blockStoreDb, err := openBlockStoreDb(args.CometHome)
			if err != nil {
				return err
			}
			defer blockStoreDb.Close()

			return migrateEvents(
				stateDb,
				blockStoreDb,
				args.MaspIndexer,
				args.StartHeight,
				args.EndHeight,
			)
		},
	}
}

func migrateEvents(
	stateDb, blockStoreDb *leveldb.DB,
	maspIndexerUrl string,
	startHeight, endHeight int,
) error {
	if maspIndexerUrl == "" {
		return fmt.Errorf("masp indexer url was not set")
	}

	if endHeight == 0 {
		lastCometHeight, err := loadLastDbHeight(blockStoreDb)
		if err != nil {
			return err
		}
		endHeight = lastCometHeight
	}

	switch {
	case startHeight <= 0:
		return fmt.Errorf("start height cannot be lower than or equal to 0")
	case endHeight < 0:
		return fmt.Errorf("end height cannot be lower than 0")
	case startHeight > endHeight:
		return fmt.Errorf(
			"start height (%d) is greater than end height (%d)",
			startHeight,
			endHeight,
		)
	}

	maspIndexerClient := NewMaspIndexerClient(maspIndexerUrl)

	wg := &sync.WaitGroup{}
	sem := make(chan struct{}, MaxConcurrentRequests)
	errs := make(chan error, 1)

	var mainErr error

	for height := startHeight; height <= endHeight; height++ {
		select {
		case mainErr = <-errs:
			break
		default:
		}

		wg.Add(1)
		sem <- struct{}{}

		go func(height int) {
			defer func() {
				<-sem
				wg.Done()
			}()

			log.Println("began processing events in block", height)

			key := []byte(fmt.Sprintf("abciResponsesKey:%d", height))
			value, err := stateDb.Get(key, &opt.ReadOptions{})
			if err != nil {
				errs <- fmt.Errorf("failed to read block %d from state db: %w", height, err)
				return
			}

			containsMaspDataRefs := false

			abciResponses := new(cmtstate.ABCIResponses)
			if err := abciResponses.Unmarshal(value); err != nil {
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
					log.Println("this db has already been migrated")
					errs <- nil
					return
				}
			}

			if !containsMaspDataRefs {
				log.Println("no masp data refs found in block", height)
				return
			}

			maspTxs, err := maspIndexerClient.BlockHeight(height)
			if err != nil {
				errs <- err
				return
			}

			namadaTxs, err := loadNamadaTxsWithMaspData(blockStoreDb, height, maspTxs)
			if err != nil {
				errs <- err
				return
			}

			for i := 0; i < len(maspTxs); i++ {
				for j := 0; j < len(maspTxs[i].Batch); j++ {
					maspTxId, err := computeMaspTxId(maspTxs[i].Batch[j].Bytes)
					if err != nil {
						errs <- fmt.Errorf("block height %d failure: %w", height, err)
						return
					}

					maspSections, err := locateMaspTxIdsInMaspSections(
						namadaTxs[maspTxs[i].BlockIndex],
					)
					if err != nil {
						errs <- fmt.Errorf("block height %d failure: %w", height, err)
						return
					}

					var sectionEventValue string

					if sec, ok := maspSections[maspTxId]; ok {
						hexHash := hex.EncodeToString(sec.Hash[:])

						if sec.Ibc {
							sectionEventValue = fmt.Sprintf(
								`{"IbcData":"%s"}`,
								hexHash,
							)

							log.Println("found ibc data section", hexHash, "at height", height, "with masp tx")
						} else {
							var buf bytes.Buffer

							err = json.NewEncoder(&buf).Encode(sec.Hash)
							errs <- fmt.Errorf(
								"block height %d failure: failed to encode masp tx id to json: %w",
								height,
								err,
							)

							sectionEventValue = fmt.Sprintf(
								`{"MaspSection":%s}`,
								buf.String(),
							)

							log.Println("found masp section", hexHash, "at height", height)
						}
					} else {
						errs <- fmt.Errorf(
							"block height %d failure: unable to locate masp tx %v",
							height,
							maspTxId,
						)
						return
					}

					abciResponses.EndBlock.Events = append(
						abciResponses.EndBlock.Events,
						types.Event{
							Type: "masp/transfer",
							Attributes: []types.EventAttribute{
								{
									Key:   "height",
									Value: strconv.Itoa(height),
									Index: true,
								},
								{
									Key: "indexed-tx",
									Value: fmt.Sprintf(
										`{"block_height":%d,"block_index":%d,"batch_index":%d}`,
										height,
										maspTxs[i].BlockIndex,
										maspTxs[i].Batch[j].MaspTxIndex,
									),
									Index: true,
								},
								{
									Key:   "section",
									Value: sectionEventValue,
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

			err = stateDb.Put(key, value, new(opt.WriteOptions))
			if err != nil {
				errs <- fmt.Errorf("failed to write migrated abci responses to db: %w", err)
			}

			log.Println("migrated all events of block", height)
		}(height)
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

	if mainErr == nil {
		log.Println("done migrating events with no errors")
	} else {
		log.Println("encountered error migrating events")
	}

	return mainErr
}

func swapRemove[T any](slice []T, index int) []T {
	slice[index] = slice[len(slice)-1]
	return slice[:len(slice)-1]
}

func decodeNamadaTxProto(protoBytes []byte) ([]byte, error) {
	var tx namproto.Tx
	err := tx.Unmarshal(protoBytes)
	if err != nil {
		return nil, fmt.Errorf("proto unmarshal of namada tx failed: %w", err)
	}
	return tx.Data, nil
}

func loadNamadaTxsWithMaspData(
	blockStoreDb *leveldb.DB,
	height int,
	maspTxs []Transaction,
) (map[int][]byte, error) {
	txs := make(map[int][]byte)

	for i := 0; i < len(maspTxs); i++ {
		block, err := loadBlock(blockStoreDb, height)
		if err != nil {
			return nil, err
		}

		namadaTx, err := decodeNamadaTxProto(block.Data.Txs[maspTxs[i].BlockIndex])
		if err != nil {
			return nil, fmt.Errorf(
				"could not parse namada tx proto bytes at height %d and index %d: %w",
				height,
				maspTxs[i].BlockIndex,
				err,
			)
		}

		txs[maspTxs[i].BlockIndex] = namadaTx
	}

	return txs, nil
}

func locateMaspTxIdsInMaspSections(
	namadaTxBorshData []byte,
) (map[[32]byte]namada.MaspTxSection, error) {
	maspTxIds, err := namada.LocateMaspTxIdsInNamTx(namadaTxBorshData)
	if err != nil {
		return nil, fmt.Errorf(
			"unable to locate masp tx ids: %w",
			err,
		)
	}
	return maspTxIds, nil
}

func computeMaspTxId(maspTxBorshData []byte) ([32]byte, error) {
	maspTxId, err := namada.ComputeMaspTxId(maspTxBorshData)
	if err != nil {
		err = fmt.Errorf(
			"unable to compute masp tx id: %w",
			err,
		)
	}
	return maspTxId, nil
}
