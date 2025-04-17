package main

import (
	"bytes"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"

	"github.com/segmentio/encoding/json"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"

	"github.com/cometbft/cometbft/abci/types"
	cmtstate "github.com/cometbft/cometbft/proto/tendermint/state"

	"github.com/heliaxdev/migrate-masp-events/namada"
	namproto "github.com/heliaxdev/migrate-masp-events/proto/types"
)

const (
	eventMaspTransfer   = "masp/transfer"
	eventMaspFeePayment = "masp/fee-payment"
)

type indexedMaspSection struct {
	blockIndex  int
	maspTxIndex int
	section     namada.MaspTxSection
}

type maspDataRefs struct {
	TxIndex  int           `json:"tx_index"`
	MaspRefs []maspDataRef `json:"masp_refs"`
}

type maspDataRef struct {
	IbcData     string
	MaspSection []byte
}

type argsMigrate struct {
	ContinueMigrating     bool
	InvalidCommitNotErr   bool
	StartHeight           int
	EndHeight             int
	CometHome             string
	MaspIndexer           string
	MaxConcurrentRequests int
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
			flags.BoolVar(
				&args.ContinueMigrating,
				"continue-migrating",
				false,
				"continue migrating the db even if we detect it's already been migrated",
			)
			flags.BoolVar(
				&args.InvalidCommitNotErr,
				"invalid-masp-commit-not-err",
				false,
				"allow using a masp indexer on an invalid commit",
			)
			flags.IntVar(
				&args.MaxConcurrentRequests,
				"max-concurrent-requests",
				DefaultMaxConcurrentRequests,
				"number of idle connections used to make concurrent requests (too many may result in connection reset by peer errors)",
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
				args.ContinueMigrating,
				args.InvalidCommitNotErr,
				args.MaxConcurrentRequests,
			)
		},
	}
}

func migrateEvents(
	stateDb, blockStoreDb *leveldb.DB,
	maspIndexerUrl string,
	startHeight, endHeight int,
	continueMigrating bool,
	invalidCommitNotErr bool,
	maxConcurrentRequests int,
) error {
	var err error

	if maspIndexerUrl == "" {
		return fmt.Errorf("masp indexer url was not set")
	}

	startHeight, endHeight, err = validateHeightRange(
		blockStoreDb,
		startHeight,
		endHeight,
	)
	if err != nil {
		return err
	}

	log.Println("migrating events starting from block", startHeight, "to", endHeight)
	totalToProcess := endHeight - startHeight + 1
	if totalToProcess > 0 {
		log.Println("processing a total of", totalToProcess, "blocks")
	} else {
		log.Println("no blocks to process")
		return nil
	}

	maspIndexerClient, err := NewMaspIndexerClient(maspIndexerUrl, maxConcurrentRequests)
	if err != nil {
		return err
	}
	if err = maspIndexerClient.ValidateVersion(invalidCommitNotErr); err != nil {
		return err
	}

	migrationCtx := newMigrateEventsSyncCtx(maxConcurrentRequests)

	var mainErr error

	stateDbTxn, err := stateDb.OpenTransaction()
	if err != nil {
		return fmt.Errorf("failed to begin state db transaction")
	}

	for height := startHeight; height <= endHeight; height++ {
		var exit bool

		if exit, mainErr = migrationCtx.checkForErrors(); exit {
			break
		}

		migrationCtx.migrateHeight(
			height,
			stateDbTxn,
			blockStoreDb,
			maspIndexerClient,
			continueMigrating,
		)
	}

	return migrationCtx.waitForAllMigrations(
		mainErr,
		stateDbTxn,
	)
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
	block, err := loadBlock(blockStoreDb, height)
	if err != nil {
		return nil, err
	}

	txs := make(map[int][]byte)

	for i := 0; i < len(maspTxs); i++ {
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

	log.Println("got all namada txs of block", height)

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

func validateHeightRange(
	blockStoreDb *leveldb.DB,
	startHeight, endHeight int,
) (int, int, error) {
	baseCometHeight, lastCometHeight, err := loadLastDbBaseAndHeight(
		blockStoreDb,
	)
	if err != nil {
		return 0, 0, err
	}

	if endHeight == 0 {
		endHeight = lastCometHeight
	}

	switch {
	case startHeight <= 0:
		return 0, 0, fmt.Errorf("start height cannot be lower than or equal to 0")
	case endHeight <= 0:
		return 0, 0, fmt.Errorf("end height cannot be lower than or equal to 0")
	case startHeight > endHeight:
		return 0, 0, fmt.Errorf(
			"start height (%d) is greater than end height (%d)",
			startHeight,
			endHeight,
		)
	}

	if baseCometHeight > startHeight || lastCometHeight < endHeight {
		return 0, 0, fmt.Errorf(
			"all comet tx data is necessary for the migrations, cannot use pruned node",
		)
	}

	return startHeight, endHeight, nil
}

type migrateEventsSyncCtx struct {
	sem  chan struct{}
	errs chan error
	wg   sync.WaitGroup
}

func newMigrateEventsSyncCtx(maxConcurrentRequests int) *migrateEventsSyncCtx {
	ctx := &migrateEventsSyncCtx{
		sem:  make(chan struct{}, maxConcurrentRequests),
		errs: make(chan error, maxConcurrentRequests+1),
	}
	return ctx
}

func (ctx *migrateEventsSyncCtx) checkForErrors() (exit bool, err error) {
	select {
	case err = <-ctx.errs:
		exit = true
	default:
	}
	return
}

func (ctx *migrateEventsSyncCtx) waitForAllMigrations(
	mainErr error,
	stateDbTxn *leveldb.Transaction,
) error {
	ctx.wg.Wait()
	close(ctx.errs)

	for freshErr := range ctx.errs {
		// NB: avoid non-errs overriding actual errs
		if freshErr != nil {
			mainErr = freshErr
		}
	}

	if mainErr == nil {
		log.Println("done migrating events with no errors")
		stateDbTxn.Commit()
	} else {
		log.Println("encountered error migrating events")
		stateDbTxn.Discard()
	}

	return mainErr
}

func (ctx *migrateEventsSyncCtx) reportErr(err error) {
	ctx.errs <- err
}

func (ctx *migrateEventsSyncCtx) migrateHeight(
	height int,
	stateDbTxn *leveldb.Transaction,
	blockStoreDb *leveldb.DB,
	maspIndexerClient *MaspIndexerClient,
	continueMigrating bool,
) {
	ctx.wg.Add(1)
	ctx.sem <- struct{}{}

	go ctx.migrateHeightTask(
		height,
		stateDbTxn,
		blockStoreDb,
		maspIndexerClient,
		continueMigrating,
	)
}

func (ctx *migrateEventsSyncCtx) releaseSem() {
	<-ctx.sem
	ctx.wg.Done()
}

func (ctx *migrateEventsSyncCtx) migrateHeightTask(
	height int,
	stateDbTxn *leveldb.Transaction,
	blockStoreDb *leveldb.DB,
	maspIndexerClient *MaspIndexerClient,
	continueMigrating bool,
) {
	defer ctx.releaseSem()

	log.Println("began processing events in block", height)

	key := []byte(fmt.Sprintf("abciResponsesKey:%d", height))
	value, err := stateDbTxn.Get(key, &opt.ReadOptions{})
	if err != nil {
		ctx.reportErr(fmt.Errorf("failed to read block %d from state db: %w", height, err))
		return
	}

	log.Println("read events of block", height, "from state db")

	var oldMaspDataRefs []indexedMaspSection

	containsNewMaspDataRefs := false

	abciResponses := new(cmtstate.ABCIResponses)
	if err := abciResponses.Unmarshal(value); err != nil {
		ctx.reportErr(fmt.Errorf("failed to unmarshal abci responses from state db: %w", err))
		return
	}

	for i := 0; i < len(abciResponses.EndBlock.Events); i++ {
		switch abciResponses.EndBlock.Events[i].Type {
		case "tx/applied":
		attributesIter:
			for e := 0; e < len(abciResponses.EndBlock.Events[i].Attributes); e++ {
				attr := abciResponses.EndBlock.Events[i].Attributes[e]

				if attr.Key == "masp_data_refs" {
					abciResponses.EndBlock.Events[i].Attributes = swapRemove(
						abciResponses.EndBlock.Events[i].Attributes,
						e,
					)

					var refs maspDataRefs

					err = json.Unmarshal([]byte(attr.Value), &refs)
					if err != nil {
						ctx.reportErr(fmt.Errorf(
							"failed to count num of old masp data refs: %w",
							err,
						))
						return
					}

					for r := 0; r < len(refs.MaspRefs); r++ {
						sec, err := refs.MaspRefs[r].toMaspTxSection()
						if err != nil {
							ctx.reportErr(fmt.Errorf(
								"failed to decode old masp data refs: %w",
								err,
							))
							return
						}

						oldMaspDataRefs = append(
							oldMaspDataRefs,
							indexedMaspSection{
								blockIndex:  refs.TxIndex,
								maspTxIndex: r,
								section:     sec,
							},
						)
					}

					break attributesIter
				}
			}
		case "masp/transfer", "masp/fee-payment":
			if !continueMigrating {
				log.Println("this db has already been migrated")
				ctx.reportErr(nil)
				return
			}
			containsNewMaspDataRefs = true
		}
	}

	if len(oldMaspDataRefs) == 0 {
		log.Println("no masp data refs found in block", height, "to migrate")
		return
	}
	if containsNewMaspDataRefs {
		ctx.reportErr(fmt.Errorf("block %d has old and new masp data refs", height))
		return
	}

	log.Println("querying events of block", height, "from masp indexer")

	maspTxs, err := maspIndexerClient.BlockHeight(height)
	if err != nil {
		ctx.reportErr(err)
		return
	}

	log.Println("loading block data of height", height, "from blockstore db")

	namadaTxs, err := loadNamadaTxsWithMaspData(blockStoreDb, height, maspTxs)
	if err != nil {
		ctx.reportErr(err)
		return
	}

	log.Println(
		"begun migrating events of block",
		height,
		"which has",
		len(maspTxs),
		"tx batches with masp txs",
	)

	var newMaspDataRefs []indexedMaspSection

	for i := 0; i < len(maspTxs); i++ {
		log.Println(
			"begun processing masp txs in tx batch",
			maspTxs[i].BlockIndex,
			"of block",
			height,
		)

		for j := 0; j < len(maspTxs[i].Batch); j++ {
			maspTxId, err := computeMaspTxId(maspTxs[i].Batch[j].Bytes)
			if err != nil {
				ctx.reportErr(fmt.Errorf("block height %d failure: %w", height, err))
				return
			}

			maspSections, err := locateMaspTxIdsInMaspSections(
				namadaTxs[maspTxs[i].BlockIndex],
			)
			if err != nil {
				ctx.reportErr(fmt.Errorf("block height %d failure: %w", height, err))
				return
			}

			log.Println(
				"located all masp sections of block",
				height,
				"at index",
				maspTxs[i].BlockIndex,
			)

			sec, ok := maspSections[maspTxId]
			if !ok {
				ctx.reportErr(fmt.Errorf(
					"block height %d failure: unable to locate masp tx %v",
					height,
					maspTxId,
				))
				return
			}

			newMaspDataRefs = append(
				newMaspDataRefs,
				indexedMaspSection{
					blockIndex:  maspTxs[i].BlockIndex,
					maspTxIndex: maspTxs[i].Batch[j].MaspTxIndex,
					section:     sec,
				},
			)
		}
	}

	if len(oldMaspDataRefs) != len(newMaspDataRefs) {
		ctx.reportErr(fmt.Errorf(
			"old masp data refs count (%d) does not match migrated refs count (%d), make sure your masp indexer endpoint is running 1.2.0",
			len(oldMaspDataRefs),
			len(newMaspDataRefs),
		))
		return
	}

	log.Println("locating masp fee payments of block", height)

	abciResponses.EndBlock.Events, err = emitMaspEvents(
		height,
		abciResponses.EndBlock.Events,
		oldMaspDataRefs,
		newMaspDataRefs,
	)
	if err != nil {
		ctx.reportErr(fmt.Errorf("failed to emit masp events: %w", err))
		return
	}

	log.Println("replaced all event data of block", height)

	value, err = abciResponses.Marshal()
	if err != nil {
		ctx.reportErr(fmt.Errorf("failed to marshal abci responses: %w", err))
		return
	}

	log.Println("storing changes of block", height, "in db")

	err = stateDbTxn.Put(key, value, new(opt.WriteOptions))
	if err != nil {
		ctx.reportErr(fmt.Errorf("failed to write migrated abci responses to db: %w", err))
	}

	log.Println("migrated all events of block", height)
}

func emitMaspEvents(
	height int,
	endBlockEvents []types.Event,
	oldMaspDataRefs, newMaspDataRefs []indexedMaspSection,
) ([]types.Event, error) {
	for newMaspDataRefIndex := 0; newMaspDataRefIndex < len(newMaspDataRefs); newMaspDataRefIndex++ {
		var (
			oldMaspDataRefIndex int
			maspEventType       string
			found               bool
		)

		// do a linear scan of the old refs to find the new event
		for ; oldMaspDataRefIndex < len(oldMaspDataRefs); oldMaspDataRefIndex++ {
			isSameSection := newMaspDataRefs[newMaspDataRefIndex].SameSection(
				&oldMaspDataRefs[oldMaspDataRefIndex],
			)
			if isSameSection {
				found = true
				break
			}
		}

		if !found {
			return nil, fmt.Errorf(
				"missing masp event at height %d: %#v",
				height,
				newMaspDataRefs[newMaspDataRefIndex],
			)
		}

		type section struct {
			BlockHeight        int
			IndexedMaspSection indexedMaspSection
		}

		if oldMaspDataRefIndex >= newMaspDataRefIndex {
			log.Println(
				"found masp fee payment:",
				fmt.Sprintf("%#v", section{
					height,
					newMaspDataRefs[newMaspDataRefIndex],
				}),
			)
			maspEventType = eventMaspFeePayment
		} else {
			log.Println(
				"found regular masp transfer:",
				fmt.Sprintf("%#v", section{
					height,
					newMaspDataRefs[newMaspDataRefIndex],
				}),
			)
			maspEventType = eventMaspTransfer
		}

		var err error

		endBlockEvents, err = appendNewMaspEvent(
			endBlockEvents,
			maspEventType,
			height,
			newMaspDataRefs[newMaspDataRefIndex].blockIndex,
			newMaspDataRefs[newMaspDataRefIndex].maspTxIndex,
			newMaspDataRefs[newMaspDataRefIndex].section,
		)
		if err != nil {
			return nil, err
		}
	}

	return endBlockEvents, nil
}

func appendNewMaspEvent(
	endBlockEvents []types.Event,
	maspEventType string,
	height int,
	blockIndex int,
	batchIndex int,
	sec namada.MaspTxSection,
) ([]types.Event, error) {
	hexHash := strings.ToUpper(hex.EncodeToString(sec.Hash[:]))

	var sectionEventValue string

	if sec.Ibc {
		sectionEventValue = fmt.Sprintf(
			`{"IbcData":"%s"}`,
			hexHash,
		)

		log.Println("stashing ibc data section", hexHash, "at height", height, "with masp tx")
	} else {
		var buf bytes.Buffer

		err := json.NewEncoder(&buf).Encode(&sec.Hash)
		if err != nil {
			return nil, fmt.Errorf(
				"block height %d failure: failed to encode masp tx id to json: %w",
				height,
				err,
			)
		}

		encoded := buf.String()
		if len(encoded) > 0 && encoded[len(encoded)-1] == '\n' {
			encoded = encoded[:len(encoded)-1]
		}

		sectionEventValue = fmt.Sprintf(
			`{"MaspSection":%s}`,
			encoded,
		)

		log.Println("stashing masp section", hexHash, "at height", height)
	}

	return append(
		endBlockEvents,
		types.Event{
			Type: maspEventType,
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
						blockIndex,
						batchIndex,
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
	), nil
}

func (s1 *indexedMaspSection) SameSection(s2 *indexedMaspSection) bool {
	return s1.section.Ibc == s2.section.Ibc && s1.section.Hash == s2.section.Hash
}

func (r *maspDataRef) toMaspTxSection() (sec namada.MaspTxSection, err error) {
	var n int

	ibcData := r.IbcData

	switch {
	case ibcData != "":
		if n = hex.DecodedLen(len(ibcData)); n != 32 {
			err = fmt.Errorf("invalid ibc data section length %d", n)
			return
		}

		n, err = hex.Decode(sec.Hash[:], []byte(ibcData))
		if n != 32 {
			err = fmt.Errorf("invalid ibc data section length %d", n)
		}
		if err != nil {
			err = fmt.Errorf("failed to decode ibc data section: %w", err)
		}

		sec.Ibc = true
	default:
		if n = len(r.MaspSection); n != 32 {
			err = fmt.Errorf("invalid masp tx id length %d", n)
			return
		}

		copy(sec.Hash[:], r.MaspSection)
	}

	return
}
