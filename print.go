package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"slices"
	"strconv"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"

	"github.com/cometbft/cometbft/abci/types"
	cmtstate "github.com/cometbft/cometbft/proto/tendermint/state"
)

type argsPrint struct {
	SkipEmpty bool
	CometHome string
}

func RegisterCommandPrint(subCommands map[string]*SubCommand) {
	subCommands["print"] = &SubCommand{
		Args:        &argsPrint{},
		Description: "print all end blocks events in the state db of cometbft",
		ConfigureFlags: func(iArgs any, flags *flag.FlagSet) {
			args := iArgs.(*argsPrint)

			flags.StringVar(&args.CometHome, "cometbft-homedir", "", "path to cometbft dir (e.g. .namada/namada.5f5de2dd1b88cba30586420/cometbft)")
			flags.BoolVar(&args.SkipEmpty, "skip-empty", false, "skip printing blocks with no events")
		},
		Entrypoint: func(iArgs any) error {
			args := iArgs.(*argsPrint)

			db, err := openStateDb(args.CometHome)
			if err != nil {
				return err
			}
			defer db.Close()

			return printEndBlocks(db, args.SkipEmpty)
		},
	}
}

func printEndBlocks(db *leveldb.DB, skipEmpty bool) error {
	iter := db.NewIterator(util.BytesPrefix([]byte("abciResponsesKey:")), &opt.ReadOptions{})
	defer iter.Release()

	var blockEvents []block

	for iter.Next() {
		abciResponses := new(cmtstate.ABCIResponses)
		err := abciResponses.Unmarshal(iter.Value())
		if err != nil {
			return fmt.Errorf("failed to unmarshal abci responses from state db: %w", err)
		}

		if skipEmpty && abciResponses.EndBlock.Events == nil {
			continue
		}

		height, err := parseHeight(extractHeight(iter.Key()))
		if err != nil {
			return err
		}

		blockEvents = append(blockEvents, block{
			BlockHeight: height,
			Events:      abciResponses.EndBlock.Events,
		})
	}

	// NB: sort data by block height
	slices.SortFunc(blockEvents, func(b1, b2 block) int {
		return b1.BlockHeight - b2.BlockHeight
	})

	writer := bufio.NewWriter(os.Stdout)
	encoder := json.NewEncoder(writer)

	err := encoder.Encode(blockEvents)
	if err != nil {
		return fmt.Errorf("failed to encode block events as json: %w", err)
	}

	return nil
}

func extractHeight(key []byte) string {
	// NB: skip "abciResponsesKey:"
	return string(key[17:])
}

func parseHeight(height string) (int, error) {
	h, err := strconv.Atoi(height)
	if err != nil {
		return 0, fmt.Errorf("failed to parse block height from db key: %w", err)
	}
	return h, nil
}

type block struct {
	BlockHeight int           `json:"block_height"`
	Events      []types.Event `json:"events"`
}
