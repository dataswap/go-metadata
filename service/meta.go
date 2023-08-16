package metaservice

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/dataswap/go-metadata/libs"
	"github.com/dataswap/go-metadata/types"
	"github.com/dataswap/go-metadata/utils"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-merkledag"
	helpers "github.com/ipfs/go-unixfs/importer/helpers"
	"github.com/ipld/go-car/util"

	"github.com/data-preservation-programs/singularity/pack"
	pi "github.com/ipfs/go-ipfs-posinfo"
	ipld "github.com/ipfs/go-ipld-format"
	dag "github.com/ipfs/go-merkledag"
	"github.com/ipfs/go-unixfs"
	pb "github.com/ipfs/go-unixfs/pb"
)

type MetaService struct {
	opts     *Options
	metas    map[cid.Cid]*types.ChunkMeta // chunks
	rawSizes map[cid.Cid]uint64           // chunks raw size
	lk       sync.Mutex

	root cid.Cid
}

func New(opts ...Option) *MetaService {
	options := newOptions(opts...)
	return &MetaService{
		opts:     options,
		root:     cid.Undef,
		metas:    make(map[cid.Cid]*types.ChunkMeta, 0),
		rawSizes: make(map[cid.Cid]uint64, 0),
	}
}

func (ms *MetaService) SetCarRoot(root cid.Cid) {
	ms.root = root
}

func (ms *MetaService) getNodeType(node ipld.Node) (pb.Data_DataType, error) {
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

func (ms *MetaService) nodeAction(node ipld.Node) *types.ChunkMeta {
	var cm types.ChunkMeta
	cm.Cid = node.Cid()
	cm.Links = node.Links()
	cm.ChunkSize = util.LdSize(node.Cid().Bytes(), node.RawData())

	if stat, err := node.Stat(); err == nil {
		//fmt.Println("hash:", stat.Hash, " link num:", stat.NumLinks, " umulativeSize ", stat.CumulativeSize, " block size", stat.BlockSize, " data size:", stat.DataSize, " raw data size:", len(node.RawData()), " ")
		cm.BlockSize = uint64(stat.BlockSize)
	}
	if nt, err := ms.getNodeType(node); err == nil {
		cm.NodeType = nt
	} else {
		fmt.Printf("get node type failed:%s\n", err.Error())
	}
	return &cm
}

func (ms *MetaService) dagServerAction(node ipld.Node) {
	cm := ms.nodeAction(node)
	ms.insertMeta(cm.Cid, cm, uint64(len(node.RawData())))
}

func (ms *MetaService) GenHelper(params *helpers.DagBuilderParams, spl libs.EnhancedSplitter) (helpers.Helper, error) {
	db, err := libs.WrappedDagBuilder(params, spl, ms.helperAction)
	if err != nil {
		return nil, err
	}

	return db, nil
}

func (ms *MetaService) helperAction(node ipld.Node, srcPath string, offset uint64, size uint64) {
	cm := ms.nodeAction(node)
	cm.SrcPath = srcPath
	cm.SrcOffset = offset
	cm.Size = size

	ms.insertMeta(cm.Cid, cm, uint64(len(node.RawData())))
}

func (ms *MetaService) GenCarWriter(w io.Writer, path string, call bool) io.Writer {
	if !call {
		return w
	}
	writer := libs.WrappedWriter(w, path, ms.carWriteAfterAction, libs.DefaultWriteBeforeAction)
	return writer
}

func (ms *MetaService) carWriteAfterAction(dstpath string, buf []byte, offset uint64) {
	///fmt.Println(">>>>>> Write dstPath:", dstpath, " offset: ", offset, " count:", len(buf))
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

	//fmt.Printf("update meta cid:%s path:%s offset:%d\n", c.String(), dstpath, offset)
	if rs, ok := ms.rawSizes[c]; ok {
		sum := rs + uint64(len(c.Bytes()))
		buf := make([]byte, 8)
		n := binary.PutUvarint(buf, sum)
		offset = offset - uint64(n)
	}
	ms.metas[c].DstOffset = offset

	return nil
}

func (ms *MetaService) SaveMeta(path string, name string) error {
	os.MkdirAll(path, 0o775)

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

	metaPath := filepath.Join(path, name)
	return utils.WriteJson(metaPath, "\t", meta)
}

func (ms *MetaService) LoadMeta(path string) error {
	var meta types.Meta
	err := utils.ReadJson(path, &meta)
	if err != nil {
		return err
	}
	ms.root = meta.DagRoot
	for _, v := range meta.Metas {
		ms.metas[v.Cid] = v
	}
	return nil
}

func (ms *MetaService) verifyMetasContinuity(metas []*types.ChunkMeta) error {
	var nextStart uint64
	for i, v := range metas {
		if i == 0 {
			nextStart = v.DstOffset
		}
		if nextStart != v.DstOffset {
			return fmt.Errorf("The chunk are damaged and are not continuous.")
		}
		nextStart = nextStart + v.ChunkSize
	}
	return nil
}

func (ms *MetaService) GetAllChunkMetas() ([]*types.ChunkMeta, error) {
	var metas []*types.ChunkMeta

	for _, v := range ms.metas {
		metas = append(metas, v)
	}

	sort.Slice(metas, func(i, j int) bool {
		return metas[i].DstOffset < metas[j].DstOffset
	})
	err := ms.verifyMetasContinuity(metas)
	return metas, err
}

func (ms *MetaService) GetChunkMetas(dstOffset uint64, dstSize uint64) ([]*types.ChunkMeta, error) {
	chunkStart := dstOffset
	chunkEnd := dstOffset + dstSize
	var metas []*types.ChunkMeta

	for _, v := range ms.metas {
		start, end := v.ChunkRange()
		if chunkStart <= start && chunkEnd >= start && chunkEnd <= end ||
			chunkStart >= start && chunkStart <= end && chunkEnd >= end ||
			chunkStart <= start && chunkEnd >= end ||
			chunkStart >= start && chunkEnd <= end {
			metas = append(metas, v)
		}
	}

	sort.Slice(metas, func(i, j int) bool {
		return metas[i].DstOffset < metas[j].DstOffset
	})

	err := ms.verifyMetasContinuity(metas)

	return metas, err
}

func (ms *MetaService) GenerateChunksFromMeta(path string, srcParent string, metas []*types.ChunkMeta) error {
	cidBuilder, err := merkledag.PrefixForCidVersion(1)
	if err != nil {
		return err
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	pack.WriteCarHeader(file, ms.root)

	for _, meta := range metas {
		var err error
		var node ipld.Node
		if meta.SrcPath != "" {
			node, err = ms.GenerateNodeFromSource(path, srcParent, meta, cidBuilder)
			if err != nil {
				return err
			}
		} else {
			node, err = ms.GenerateNodeNoData(meta, cidBuilder)
			if err != nil {
				return err
			}
		}

		if node.Cid().String() != meta.Cid.String() {
			return fmt.Errorf("The generated CID for the node is not consistent with the metadata record.")
		}

		//fmt.Println("gen file cid:", node.Cid().String(), " meta cid:", meta.Cid.String())

		_, err = file.Seek(int64(meta.DstOffset), 0)
		if err != nil {
			return err
		}
		pack.WriteCarBlock(file, node)
	}

	return nil
}

func (ms *MetaService) GenerateNodeNoData(meta *types.ChunkMeta, cidBuilder cid.Builder) (ipld.Node, error) {
	fsNode := helpers.NewFSNodeOverDag(meta.NodeType, cidBuilder)
	for _, link := range meta.Links {
		cm, ok := ms.metas[link.Cid]
		if !ok {
			return nil, fmt.Errorf("cant find meta ,cid:%s", link.Cid.String())
		}
		var blockSize uint64
		if meta.NodeType == pb.Data_File {
			blockSize = cm.Size
		} else {
			blockSize = cm.BlockSize
		}
		if err := fsNode.AddLinkChildToFsNode(link, blockSize); err != nil {
			return nil, err
		}
		//fmt.Println("AddChildLink name:", link.Name, " cid:", link.Cid.String(), " file size:", blockSize, " meta")
	}
	node, err := fsNode.Commit()
	if err != nil {
		return nil, err
	}
	return node, nil
}

func (ms *MetaService) GenerateNodeFromSource(path string, srcParent string, meta *types.ChunkMeta, cidBuilder cid.Builder) (ipld.Node, error) {
	srcPath := filepath.Join(srcParent, meta.SrcPath)
	sfile, err := os.Open(srcPath)
	if err != nil {
		return nil, err
	}
	defer sfile.Close()
	data := make([]byte, meta.Size)

	if _, err := sfile.ReadAt(data, int64(meta.SrcOffset)); err != nil {
		return nil, err
	}

	node, err := helpers.NewLeafNode(data, meta.NodeType, cidBuilder, ms.opts.rawLeaves)
	if err != nil {
		return nil, err
	}

	node = helpers.ProcessFileStore(node, meta.Size)

	return node, nil
}
