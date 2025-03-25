package main

import (
	"fmt"
	"path/filepath"

	"github.com/syndtr/goleveldb/leveldb"
)

func openStateDb(cometHome string) (*leveldb.DB, error) {
	if cometHome == "" {
		return nil, fmt.Errorf("no cometbft home dir provided as arg")
	}

	dbPath := filepath.Join(cometHome, "data", "state.db")

	db, err := leveldb.OpenFile(dbPath, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open db in %s: %w", dbPath, err)
	}

	return db, nil
}
