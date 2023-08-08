package service

import (
	"fmt"
	"io"
	"math"
	"sync"

	"github.com/dataswap/go-metadata/libs"
	"github.com/dataswap/go-metadata/types"
	"github.com/ipfs/go-cid"
	chunker "github.com/ipfs/go-ipfs-chunker"
	helpers "github.com/ipfs/go-unixfs/importer/helpers"
	"github.com/ipfs/go-unixfsnode/data"
	dagpb "github.com/ipld/go-codec-dagpb"
	"github.com/multiformats/go-multicodec"

	pb "github.com/ipfs/go-unixfs/pb"

	ipld "github.com/ipfs/go-ipld-format"
)

const (
	DefaultMaxCommpBuffSizePad  = uint64(1 << 20)
	DefaultMaxCommpBuffSize     = uint64(DefaultMaxCommpBuffSizePad - (DefaultMaxCommpBuffSizePad / 128))
	DefaultMerkleLeavesNodeSize = 127 * 256 * 32 * 2
)

var DefaultMerkletreeStartNumberOfLayers = uint(math.Log2(float64(DefaultMerkleLeavesNodeSize)/32) + 1)

type MetaService struct {
	spl    chunker.Splitter //Splitter
	writer io.Writer        //car's writer
	helper helpers.Helper   //Helper
	ds     ipld.DAGService

	metas map[cid.Cid]*types.ChunkMeta // chunks
	lk    sync.Mutex

	splCh chan *types.SrcData // source data slice channel

	//calc  *commp.Calc             //commp calc
	hashs map[uint]map[int][]byte //layer -> node index -> hash
	hlk   sync.Mutex
}

func New() *MetaService {
	return &MetaService{
		metas: make(map[cid.Cid]*types.ChunkMeta, 0),
		splCh: make(chan *types.SrcData),
		hashs: make(map[uint]map[int][]byte),
	}
}

func (ms *MetaService) getNodeType(node ipld.Node) (pb.Data_DataType, error) {
	var nodeType pb.Data_DataType
	if node.Cid().Prefix().Codec == uint64(multicodec.DagPb) {
		builder := dagpb.Type.PBNode.NewBuilder()
		if err := dagpb.DecodeBytes(builder, node.RawData()); err != nil {
			return nodeType, err
		}
		n := builder.Build()
		pbn, ok := n.(dagpb.PBNode)
		if !ok {
			return nodeType, fmt.Errorf("node cant assert to PBNode")
		}
		ufd, err := data.DecodeUnixFSData(pbn.Data.Must().Bytes())
		if err != nil {
			return nodeType, err
		}
		nodeType = pb.Data_DataType(ufd.FieldDataType().Int())
	}
	return nodeType, nil
}

func (ms *MetaService) dagServerAction(node ipld.Node) {
	if nt, err := ms.getNodeType(node); err != nil {
		var cm types.ChunkMeta
		meta := <-ms.splCh
		{
			cm.SrcPath = meta.Path
			cm.SrcOffset = meta.Offset
			cm.Size = meta.Size
			cm.NodeType = nt
			cm.Cid = node.Cid()
			cm.Links = node.Links()
		}
		ms.insertMeta(cm.Cid, &cm)
	}
}

func (ms *MetaService) SetHelper(params *helpers.DagBuilderParams, spl chunker.Splitter) (helpers.Helper, error) {
	db, err := libs.WrappedDagBuilder(params, spl, ms.helperAction)
	if err != nil {
		return nil, err
	}
	ms.helper = db
	return db, nil
}

func (ms *MetaService) helperAction(node ipld.Node, nodeType pb.Data_DataType) {
	var cm types.ChunkMeta
	meta := <-ms.splCh
	{
		cm.SrcPath = meta.Path
		cm.SrcOffset = meta.Offset
		cm.Size = meta.Size
		cm.NodeType = nodeType
		cm.Cid = node.Cid()
		cm.Links = node.Links()
	}

	ms.insertMeta(cm.Cid, &cm)
}

func (ms *MetaService) SetSplitter(r io.Reader, srcPath string, call bool) chunker.Splitter {
	spl := libs.NewSliceSplitter(r, int64(libs.UnixfsChunkSize), srcPath, ms.splitterAction, call)
	ms.spl = spl
	return spl
}

func (ms *MetaService) splitterAction(srcPath string, offset uint64, size uint32, eof bool) {
	go func() {
		ms.splCh <- &types.SrcData{
			Path:   srcPath,
			Offset: offset,
			Size:   size,
		}
	}()
}

func (ms *MetaService) SetCarWriter(w io.Writer, path string, call bool) io.Writer {
	if !call {
		return w
	}
	writer := libs.WrappedWriter(w, path, ms.carWriteAfterAction, libs.DefaultWriteBeforeAction)
	ms.writer = writer
	return writer
}

func (ms *MetaService) carWriteAfterAction(dstpath string, c cid.Cid, count int, offset uint64) {
	if _, ok := ms.metas[c]; !ok {
		fmt.Printf("meta cid: %s is not exist\n", c.String())
		return
	}
	ms.updateMeta(c, dstpath, offset)
}

func (ms *MetaService) insertMeta(c cid.Cid, cm *types.ChunkMeta) error {
	ms.lk.Lock()
	defer ms.lk.Unlock()
	if _, ok := ms.metas[c]; ok {
		return fmt.Errorf("meta srcpath:%s offset: %d size: %d cid: %s exist", cm.SrcPath, cm.SrcOffset, cm.Size, c.String())
	}
	ms.metas[c] = cm
	return nil
}

func (ms *MetaService) updateMeta(c cid.Cid, dstpath string, offset uint64) error {
	ms.lk.Lock()
	defer ms.lk.Unlock()
	if _, ok := ms.metas[c]; !ok {
		return fmt.Errorf("meta cid: %s is not exist", c.String())
	}

	ms.metas[c].DstPath = dstpath
	ms.metas[c].DstOffset = offset

	return nil
}
