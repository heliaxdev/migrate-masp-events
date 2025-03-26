package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"slices"
	"sync"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"

	"github.com/cometbft/cometbft/abci/types"
	cmtstate "github.com/cometbft/cometbft/proto/tendermint/state"

	namproto "github.com/heliaxdev/migrate-masp-events/proto/types"
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

			return migrateEvents(stateDb, blockStoreDb, args.MaspIndexer)
		},
	}
}

func migrateEvents(stateDb, blockStoreDb *leveldb.DB, maspIndexerUrl string) error {
	if maspIndexerUrl == "" {
		return fmt.Errorf("masp indexer url was not set")
	}

	maspIndexerClient := NewMaspIndexerClient(maspIndexerUrl)

	iter := stateDb.NewIterator(util.BytesPrefix([]byte("abciResponsesKey:")), &opt.ReadOptions{})
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

			//namadaTxs, err := loadNamadaTxs(blockStoreDb, height, maspTxs)
			//if err != nil {
			//	errs <- err
			//	return
			//}

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

			err = stateDb.Put(key, value, new(opt.WriteOptions))
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

func decodeNamadaTxProto(protoBytes []byte) ([]byte, error) {
	var tx namproto.Tx
	err := tx.Unmarshal(protoBytes)
	if err != nil {
		return nil, fmt.Errorf("proto unmarshal of namada tx failed: %w", err)
	}
	return tx.Data, nil
}

func loadNamadaTxs(blockStoreDb *leveldb.DB, height int, maspTxs []Transaction) ([][]byte, error) {
	var txs [][]byte

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

		txs = append(txs, namadaTx)
	}

	return txs, nil
}

func locateMaspTxIdsInMaspSections(namadaTxBorshData []byte) ([][32]byte, error) {
	maspTxIds, err := locateMaspTxIdsInMaspSectionsAux(namadaTxBorshData)
	if err != nil {
		return nil, fmt.Errorf(
			"unable to localte masp tx ids: failed to parse borsh encoded namada tx: %w",
			err,
		)
	}
	return maspTxIds, nil
}

func locateMaspTxIdsInMaspSectionsAux(namadaTxBorshData []byte) ([][32]byte, error) {
	var (
		err            error
		parsedSections [][32]byte
	)

	p := namadaTxBorshData

	// PARSE TX HEADER
	// =========================================================================

	// skip chain id
	p, err = borshSkipCollection("chain_id:ChainId", p)
	if err != nil {
		return nil, err
	}

	// skip expiration
	p, err = borshOption("expiration:Option<DateTimeUtc>", p, func(p []byte) ([]byte, error) {
		return borshSkipCollection("expiration:DateTimeUtc", p)
	})
	if err != nil {
		return nil, err
	}

	// skip timestamp
	p, err = borshSkipCollection("timestamp:DateTimeUtc", p)
	if err != nil {
		return nil, err
	}

	// skip commitments
	p, err = borshSkipCollection("batch:HashSet<TxCommitments>", p)
	if err != nil {
		return nil, err
	}

	// skip atomic flag
	p, err = borshSkipBool("atomic:bool", p)
	if err != nil {
		return nil, err
	}

	// skip tx type
	p, err = borshSkipTxType("tx_type:TxType", p)
	if err != nil {
		return nil, err
	}

	// PARSE TX SECTIONS
	// =========================================================================
	numHeaders, err := borshReadLength("sections:Vec<Header>", p)
	if err != nil {
		return nil, err
	}

	for i := 0; i < numHeaders; i++ {
		newP, maspTxId, ok, err := borshMaybeMaspSection("section:Section", p)
		if err != nil {
			return nil, err
		}

		p = newP

		if !ok {
			continue
		}

		parsedSections = append(parsedSections, maspTxId)
	}

	return parsedSections, nil
}

func borshMaybeMaspSection(descriptor string, p []byte) ([]byte, [32]byte, bool, error) {
	// TODO
	return nil, [32]byte{}, false, nil
}

func borshSkipTxType(descriptor string, p []byte) ([]byte, error) {
	if len(p) < 1 {
		return nil, fmt.Errorf("error decoding %s: TxType discriminant not present", descriptor)
	}

	switch p[0] {
	// raw tx
	case 0:
		return p[1:], nil
	// wrapper tx
	case 1:
		const (
			denominatedAmountLen = 256 + 1
			addressLen           = 1 + 20
			feeLen               = denominatedAmountLen + addressLen
			pubKeyLenSecp        = 1 + 33
			pubKeyLenEdwrd       = 1 + 32
			gasLimitLen          = 8
			wrapperTxLenSecp     = feeLen + pubKeyLenSecp + gasLimitLen
			wrapperTxLenEdwrd    = feeLen + pubKeyLenEdwrd + gasLimitLen
		)
		if len(p) < feeLen+1 {
			return nil, fmt.Errorf("error decoding %s: incomplete wrapper tx data in header", descriptor)
		}
		var wrapperTxLen int
		switch p[feeLen] {
		case 0:
			wrapperTxLen = wrapperTxLenEdwrd
		case 1:
			wrapperTxLen = wrapperTxLenSecp
		default:
			return nil, fmt.Errorf("error decoding %s: invalid pubkey in header", descriptor)
		}
		return p[wrapperTxLen:], nil
	// protocol tx
	case 2:
		return nil, fmt.Errorf("error decoding %s: there should be no protocol txs in blocks")
	default:
		return nil, fmt.Errorf("error decoding %s: invalid TxType discriminant %d", descriptor, p[0])
	}
}

func borshSkipBool(descriptor string, p []byte) ([]byte, error) {
	if len(p) < 1 {
		return nil, fmt.Errorf("error decoding %s: bool is not present", descriptor)
	}
	return p[1:], nil
}

func borshOption(descriptor string, p []byte, f func([]byte) ([]byte, error)) ([]byte, error) {
	if len(p) < 1 {
		return nil, fmt.Errorf("error decoding %s: option tag byte is not present", descriptor)
	}

	if p[0] == 0 {
		return p[1:], nil
	}

	return f(p[1:])
}

func borshSkipCollection(descriptor string, itemLen int, p []byte) ([]byte, error) {
	n, err := borshReadLength(descriptor, p)
	if err != nil {
		return nil, err
	}
	p = p[4:]

	skipLen := n * itemLen
	if len(p) < skipLen {
		return 0, fmt.Errorf("error decoding %s: short collection", descriptor)
	}

	return p[skipLen:], nil
}

func borshReadLength(descriptor string, p []byte) (int, error) {
	if len(p) < 4 {
		return 0, fmt.Errorf("error decoding %s: could not read length of collection", descriptor)
	}
	return int(binary.LittleEndian.Uint32(p)), nil
}
