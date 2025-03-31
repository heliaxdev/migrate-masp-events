package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"

	"github.com/cosmos/gogoproto/proto"

	cmtstore "github.com/cometbft/cometbft/proto/tendermint/store"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	cmt "github.com/cometbft/cometbft/types"
)

func dirExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return info.IsDir(), nil
}

func openStateDb(cometHome string) (*leveldb.DB, error) {
	return openDb(cometHome, "state.db", false)
}

func openBlockStoreDb(cometHome string) (*leveldb.DB, error) {
	return openDb(cometHome, "blockstore.db", true)
}

func openDb(cometHome, dbName string, readOnly bool) (*leveldb.DB, error) {
	if cometHome == "" {
		return nil, fmt.Errorf("no cometbft home dir provided as arg")
	}

	dbPath := filepath.Join(cometHome, "data", dbName)
	ok, err := dirExists(dbPath)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to open db %s in %s: %w",
			dbName,
			cometHome,
			err,
		)
	}
	if !ok {
		return nil, fmt.Errorf(
			"failed to open db %s in %s: leveldb db does not exist",
			dbName,
			cometHome,
		)
	}

	db, err := leveldb.OpenFile(dbPath, &opt.Options{
		ErrorIfMissing: true,
		ReadOnly:       readOnly,
	})
	if err != nil {
		return nil, fmt.Errorf(
			"failed to open db %s in %s: %w",
			dbName,
			cometHome,
			err,
		)
	}

	return db, nil
}

func loadBlock(blockStore *leveldb.DB, height int) (*cmt.Block, error) {
	blockMeta, err := loadBlockMeta(blockStore, height)
	if err != nil {
		return nil, err
	}
	if blockMeta == nil {
		return nil, nil
	}

	pbb := new(cmtproto.Block)
	buf := make([]byte, 0, 4096)

	log.Println(
		"reading",
		blockMeta.BlockID.PartSetHeader.Total,
		"parts of block",
		height,
	)

	for i := 0; i < int(blockMeta.BlockID.PartSetHeader.Total); i++ {
		part, err := loadBlockPart(blockStore, height, i)
		if err != err {
			return nil, err
		}
		buf = append(buf, part.Bytes...)
	}

	log.Println("unmarshaling block", height)

	err = proto.Unmarshal(buf, pbb)
	if err != nil {
		return nil, fmt.Errorf(
			"unmarshal of block %d from parts failed: %w",
			height,
			err,
		)
	}

	block, err := cmt.BlockFromProto(pbb)
	if err != nil {
		return nil, fmt.Errorf(
			"error from proto block %d: %w",
			height,
			err,
		)
	}

	log.Println("all data of block", height, "retrieved")

	return block, nil
}

func loadBlockPart(blockStore *leveldb.DB, height, index int) (*cmt.Part, error) {
	pbpart := new(cmtproto.Part)

	log.Println("reading part", index, "of block", height)

	bz, err := blockStore.Get(
		[]byte(fmt.Sprintf("P:%d:%d", height, index)),
		new(opt.ReadOptions),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to read block part %d of height %d from db: %w",
			index,
			height,
			err,
		)
	}

	err = proto.Unmarshal(bz, pbpart)
	if err != nil {
		return nil, fmt.Errorf(
			"block unmarshal to cmtproto.Part of part %d of height %d failed: %w",
			index,
			height,
			err,
		)
	}

	part, err := cmt.PartFromProto(pbpart)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to get block part %d of height %d from proto: %w",
			index,
			height,
			err,
		)
	}

	log.Println("got part", index, "of block", height)

	return part, nil
}

func loadBlockMeta(blockStore *leveldb.DB, height int) (*cmt.BlockMeta, error) {
	pbbm := new(cmtproto.BlockMeta)

	bz, err := blockStore.Get(
		[]byte(fmt.Sprintf("H:%d", height)),
		new(opt.ReadOptions),
	)
	if err != nil {
		if errors.Is(err, leveldb.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf(
			"failed to read block meta of height %d from db: %w",
			height,
			err,
		)
	}

	err = proto.Unmarshal(bz, pbbm)
	if err != nil {
		return nil, fmt.Errorf("unmarshal to cmtproto.BlockMeta: %w", err)
	}

	blockMeta, err := cmt.BlockMetaFromTrustedProto(pbbm)
	if err != nil {
		return nil, fmt.Errorf("error from proto blockMeta: %w", err)
	}

	return blockMeta, nil
}

func loadLastDbBaseAndHeight(blockStoreDb *leveldb.DB) (int, int, error) {
	value, err := blockStoreDb.Get([]byte("blockStore"), &opt.ReadOptions{})
	if err != nil {
		return 0, 0, fmt.Errorf(
			"failed to read last committed height from blockstore db: %w",
			err,
		)
	}

	var state cmtstore.BlockStoreState
	if err := proto.Unmarshal(value, &state); err != nil {
		return 0, 0, fmt.Errorf(
			"could not deserialize latest height from blockstore db: %w",
			err,
		)
	}

	return int(state.Base), int(state.Height), nil
}

func loadLastDbHeight(blockStoreDb *leveldb.DB) (int, error) {
	_, height, err := loadLastDbBaseAndHeight(blockStoreDb)
	if err != nil {
		return 0, err
	}
	return height, err
}
