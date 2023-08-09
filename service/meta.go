package metaservice

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"sort"
	"sync"

	"github.com/dataswap/go-metadata/libs"
	"github.com/dataswap/go-metadata/types"
	"github.com/dataswap/go-metadata/utils"
	"github.com/ipfs/go-cid"
	chunker "github.com/ipfs/go-ipfs-chunker"
	helpers "github.com/ipfs/go-unixfs/importer/helpers"
	"github.com/ipld/go-car/util"

	pi "github.com/ipfs/go-ipfs-posinfo"
	ipld "github.com/ipfs/go-ipld-format"
	dag "github.com/ipfs/go-merkledag"
	"github.com/ipfs/go-unixfs"
	pb "github.com/ipfs/go-unixfs/pb"
)

const (
	DefaultMaxCommpBuffSizePad  = uint64(1 << 20)
	DefaultMaxCommpBuffSize     = uint64(DefaultMaxCommpBuffSizePad - (DefaultMaxCommpBuffSizePad / 128))
	DefaultMerkleLeavesNodeSize = 127 * 256 * 32 * 2
)

var DefaultMerkletreeStartNumberOfLayers = uint(math.Log2(float64(DefaultMerkleLeavesNodeSize)/32) + 1)

type MetaService struct {
	opts   *Options
	helper helpers.Helper //Helper

	metas    map[cid.Cid]*types.ChunkMeta // chunks
	rawSizes map[cid.Cid]uint64           // chunks raw size
	lk       sync.Mutex
	root     cid.Cid

	splCh chan *types.SrcData // source data slice channel

	//calc  *commp.Calc             //commp calc
	hashs map[uint]map[int][]byte //layer -> node index -> hash
	hlk   sync.Mutex
}

func New(opts ...Option) *MetaService {
	options := newOptions(opts...)
	return &MetaService{
		opts:     options,
		root:     cid.Undef,
		metas:    make(map[cid.Cid]*types.ChunkMeta, 0),
		rawSizes: make(map[cid.Cid]uint64, 0),
		splCh:    make(chan *types.SrcData),
		hashs:    make(map[uint]map[int][]byte),
	}
}

func (ms *MetaService) SetCarRoot(root cid.Cid) {
	ms.root = root
}

func (ms *MetaService) getNodeType(node ipld.Node) (pb.Data_DataType, error) {
	//fmt.Println("node type:", reflect.TypeOf(node))
	switch tnode := node.(type) {
	case *pi.FilestoreNode:
		{
			switch fnode := tnode.Node.(type) {
			case *dag.ProtoNode:
				return ms.getProtoNodeType(fnode)
			case *dag.RawNode:
				return pb.Data_Raw, nil
			default:
				return 0xff, unixfs.ErrUnrecognizedType
			}
		}
	case *dag.ProtoNode:
		return ms.getProtoNodeType(tnode)
	default:
		return 0xff, unixfs.ErrUnrecognizedType
	}
}

func (ms *MetaService) getProtoNodeType(node *dag.ProtoNode) (pb.Data_DataType, error) {
	fsNode, err := unixfs.FSNodeFromBytes(node.Data())
	if err != nil {
		return 0xff, fmt.Errorf("incorrectly formatted protobuf: %s", err)
	}
	return fsNode.Type(), nil
}

func (ms *MetaService) GenDagService(ds ipld.DAGService) ipld.DAGService {
	return libs.WrappedDagService(ds, ms.dagServerAction)
}

func (ms *MetaService) dagServerAction(node ipld.Node) {
	var cm types.ChunkMeta
	cm.Cid = node.Cid()
	cm.Links = node.Links()
	cm.ChunkSize = util.LdSize(node.Cid().Bytes(), node.RawData())
	if nt, err := ms.getNodeType(node); err == nil {
		cm.NodeType = nt
	} else {
		fmt.Printf("get node type failed:%s\n", err.Error())
	}
	select {
	case meta := <-ms.splCh:
		{
			cm.SrcPath = meta.Path
			cm.SrcOffset = meta.Offset
			cm.Size = meta.Size
			//fmt.Println("meta size: ", meta.Size, " node size: ", size)
		}
	default:
	}
	ms.insertMeta(cm.Cid, &cm, uint64(len(node.RawData())))
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

	ms.insertMeta(cm.Cid, &cm, uint64(len(node.RawData())))
}

func (ms *MetaService) GenSplitter(r io.Reader, srcPath string, call bool) chunker.Splitter {
	return libs.NewSliceSplitter(r, int64(libs.UnixfsChunkSize), srcPath, ms.splitterAction, call)
}

func (ms *MetaService) splitterAction(srcPath string, offset uint64, size uint32, eof bool) {
	go func() {
		ms.splCh <- &types.SrcData{
			Path:   srcPath,
			Offset: offset,
			Size:   uint64(size),
		}
	}()
}

func (ms *MetaService) GenCarWriter(w io.Writer, path string, call bool) io.Writer {
	if !call {
		return w
	}
	writer := libs.WrappedWriter(w, path, ms.carWriteAfterAction, libs.DefaultWriteBeforeAction)
	return writer
}

func (ms *MetaService) carWriteAfterAction(dstpath string, buf []byte, offset uint64) {
	fmt.Println(">>>>>> Write dstPath:", dstpath, " offset: ", offset, " count:", len(buf))

	if c, err := cid.Parse(buf); err == nil {
		if _, ok := ms.metas[c]; !ok {
			fmt.Printf("meta cid: %s is not exist\n", c.String())
			return
		}
		if err := ms.updateMeta(c, dstpath, offset); err != nil {
			fmt.Printf("update meta failed:%s\n", err.Error())
		}
	}
}

func (ms *MetaService) insertMeta(c cid.Cid, cm *types.ChunkMeta, rawSize uint64) error {
	ms.lk.Lock()
	defer ms.lk.Unlock()
	if _, ok := ms.metas[c]; ok {
		return fmt.Errorf("meta srcpath:%s offset: %d size: %d cid: %s exist", cm.SrcPath, cm.SrcOffset, cm.Size, c.String())
	}
	ms.metas[c] = cm
	ms.rawSizes[c] = rawSize
	return nil
}

func (ms *MetaService) updateMeta(c cid.Cid, dstpath string, offset uint64) error {
	ms.lk.Lock()
	defer ms.lk.Unlock()
	if _, ok := ms.metas[c]; !ok {
		return fmt.Errorf("meta cid: %s is not exist", c.String())
	}

	fmt.Printf("update meta cid:%s path:%s offset:%d\n", c.String(), dstpath, offset)
	//ms.metas[c].DstPath = dstpath
	if rs, ok := ms.rawSizes[c]; ok {
		sum := rs + uint64(len(c.Bytes()))
		buf := make([]byte, 8)
		n := binary.PutUvarint(buf, sum)
		offset = offset - uint64(n)
	}
	ms.metas[c].DstOffset = offset

	return nil
}

func (ms *MetaService) PrintJson(path string) error {
	meta := &types.Meta{
		DagRoot: ms.root,
		Metas:   make([]*types.ChunkMeta, 0),
	}
	ms.lk.Lock()
	defer ms.lk.Unlock()
	for _, v := range ms.metas {
		meta.Metas = append(meta.Metas, v)
	}

	sort.Slice(meta.Metas, func(i int, j int) bool {
		return meta.Metas[i].DstOffset < meta.Metas[j].DstOffset
	})

	return utils.WriteJson(path+"/"+ms.root.String()+".json", "\t", meta)
}
