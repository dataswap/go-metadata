package service

import (
	"io"
	"sync"

	"github.com/dataswap/go-metadata/types"
	"github.com/ipfs/go-cid"
	chunker "github.com/ipfs/go-ipfs-chunker"
	helpers "github.com/ipfs/go-unixfs/importer/helpers"
)

type MetaService struct {
	spl    chunker.Splitter //Splitter
	writer io.Writer        //car's writer
	helper helpers.Helper   //Helper

	metas map[cid.Cid]*types.ChunkMeta // chunks
	lk    sync.Mutex

	splCh chan *types.SrcData // source data slice channel

	//calc  *commp.Calc             //commp calc
	hashs map[uint]map[int][]byte //layer -> node index -> hash
	hlk   sync.Mutex
}
