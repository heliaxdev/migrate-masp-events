package main

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"

	"github.com/cosmos/gogoproto/proto"

	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	cmt "github.com/cometbft/cometbft/types"
)

func openStateDb(cometHome string) (*leveldb.DB, error) {
	return openDb(cometHome, "state.db")
}

func openBlockStoreDb(cometHome string) (*leveldb.DB, error) {
	return openDb(cometHome, "blockstore.db")
}

func openDb(cometHome, dbName string) (*leveldb.DB, error) {
	if cometHome == "" {
		return nil, fmt.Errorf("no cometbft home dir provided as arg")
	}

	dbPath := filepath.Join(cometHome, "data", dbName)

	db, err := leveldb.OpenFile(dbPath, nil)
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
	buf := make([]byte, 0, 1024)

	for i := 0; i < int(blockMeta.BlockID.PartSetHeader.Total); i++ {
		part, err := loadBlockPart(blockStore, height, i)
		if err != err {
			return nil, err
		}
		buf = append(buf, part.Bytes...)
	}

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

	return block, nil
}

func loadBlockPart(blockStore *leveldb.DB, height, index int) (*cmt.Part, error) {
	pbpart := new(cmtproto.Part)

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

	if len(bz) == 0 {
		return nil, fmt.Errorf("block %d is not committed", height)
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
