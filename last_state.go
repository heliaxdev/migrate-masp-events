package main

import (
	"flag"
	"fmt"

	"github.com/syndtr/goleveldb/leveldb"
)

type argsLastState struct {
	CometHome string
}

func RegisterCommandLastState(subCommands map[string]*SubCommand) {
	subCommands["last-state"] = &SubCommand{
		Args:        &argsLastState{},
		Description: "print last block state in the blockstore db of cometbft",
		ConfigureFlags: func(iArgs any, flags *flag.FlagSet) {
			args := iArgs.(*argsLastState)

			flags.StringVar(&args.CometHome, "cometbft-homedir", "", "path to cometbft dir (e.g. .namada/namada.5f5de2dd1b88cba30586420/cometbft)")
		},
		Entrypoint: func(iArgs any) error {
			args := iArgs.(*argsLastState)

			db, err := openBlockStoreDb(args.CometHome)
			if err != nil {
				return err
			}
			defer db.Close()

			return printLastBlockState(db)
		},
	}
}

func printLastBlockState(db *leveldb.DB) error {
	base, height, err := loadLastDbBaseAndHeight(db)
	if err != nil {
		return err
	}

	fmt.Println(
		"Base height is",
		base,
		"and latest height is",
		height,
	)

	return nil
}
