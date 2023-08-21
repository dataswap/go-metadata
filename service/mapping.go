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

const (
	METAS_PATH          = "metas"
	MAPPINGS_PATH       = "mappings"
	MAPPING_FILE_SUFFIX = ".json"
)

// MappingService generates the mapping relationship from the source file to the car file.
type MappingService struct {
	opts         *Options
	mappings     map[cid.Cid]*types.ChunkMapping // chunks
	chunkRawSize map[cid.Cid]uint64              // chunks raw size
	lk           sync.Mutex

	dataRoot cid.Cid
}

func New(opts ...Option) *MappingService {
	options := newOptions(opts...)
	return &MappingService{
		opts:         options,
		dataRoot:     cid.Undef,
		mappings:     make(map[cid.Cid]*types.ChunkMapping, 0),
		chunkRawSize: make(map[cid.Cid]uint64, 0),
	}
}

// Singleton pattern for MappingService, used for data sampling restoration, proof generation, and verification.
var (
	instance *MappingService
	once     sync.Once
)

func MappingServiceInstance(opts ...Option) *MappingService {
	once.Do(func() {
		instance = New(opts...)
	})
	return instance
}

// Set the DAG data root for the current CAR file.
func (ms *MappingService) SetCarDataRoot(root cid.Cid) {
	ms.dataRoot = root
}

// Get the Data_DataType of a node.
func (ms *MappingService) getNodeType(node ipld.Node) (pb.Data_DataType, error) {
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

// Get the Data_DataType of a node of type ProtoNode.
func (ms *MappingService) getProtoNodeType(node *dag.ProtoNode) (pb.Data_DataType, error) {
	fsNode, err := unixfs.FSNodeFromBytes(node.Data())
	if err != nil {
		return 0xff, fmt.Errorf("incorrectly formatted protobuf: %s", err)
	}
	return fsNode.Type(), nil
}

// Creating an encapsulation interface for ipld.DAGService to implement recording of mapping files during the process of writing nodes
func (ms *MappingService) GenerateDagService(ds ipld.DAGService) ipld.DAGService {
	return libs.WrappedDagService(ds, ms.dagServerAction)
}

// Getting node information from the ipld.DAGService interface during node writes
func (ms *MappingService) nodeAction(node ipld.Node) *types.ChunkMapping {
	var cm types.ChunkMapping
	cm.Cid = node.Cid()
	cm.Links = node.Links()
	cm.ChunkSize = util.LdSize(node.Cid().Bytes(), node.RawData())

	if stat, err := node.Stat(); err == nil {
		cm.BlockSize = uint64(stat.BlockSize)
	}
	if nt, err := ms.getNodeType(node); err == nil {
		cm.NodeType = nt
	} else {
		fmt.Printf("get node type failed:%s\n", err.Error())
	}
	return &cm
}

// Callback entry point for ipld.DAGService,get node information and writing it to a cache
func (ms *MappingService) dagServerAction(node ipld.Node) {
	cm := ms.nodeAction(node)
	ms.insertMapping(cm.Cid, cm, uint64(len(node.RawData())))
}

// Implementing the Helper interface of go-unixfs.
func (ms *MappingService) GenerateHelper(params *helpers.DagBuilderParams, spl libs.EnhancedSplitter) (helpers.Helper, error) {
	db, err := libs.WrappedDagBuilder(params, spl, ms.helperAction)
	if err != nil {
		return nil, err
	}

	return db, nil
}

// Callback entry point for helpers.Helper, get the source file's path, offset, and write size information when constructing a DAG node.
// This function is only called when nodes are directly generated from the source file.
func (ms *MappingService) helperAction(node ipld.Node, srcPath string, offset uint64, size uint64) {
	cm := ms.nodeAction(node)
	cm.SrcPath = srcPath
	cm.SrcOffset = offset
	cm.Size = size

	ms.insertMapping(cm.Cid, cm, uint64(len(node.RawData())))
}

// Wrapping the io.Writer interface to implement mapping recording for writing CAR files.
func (ms *MappingService) GenerateCarWriter(w io.Writer, path string, call bool) io.Writer {
	if !call {
		return w
	}
	writer := libs.WrappedWriter(w, path, ms.carWriteAfterAction, libs.DefaultWriteBeforeAction)
	return writer
}

// This callback function is invoked after each node is written to the CAR file,
// recording the offset and size of the written data in the CAR file.
func (ms *MappingService) carWriteAfterAction(dstpath string, buf []byte, offset uint64) {
	// Mapping file updates are only executed when data can be parsed as a node during the writing process.
	if c, err := cid.Parse(buf); err == nil {
		if _, ok := ms.mappings[c]; !ok {
			fmt.Printf("meta cid: %s is not exist\n", c.String())
			return
		}
		if err := ms.updateMapping(c, offset); err != nil {
			fmt.Printf("update meta failed:%s\n", err.Error())
		}
	}
}

func (ms *MappingService) insertMapping(c cid.Cid, cm *types.ChunkMapping, rawSize uint64) error {
	ms.lk.Lock()
	defer ms.lk.Unlock()
	if _, ok := ms.mappings[c]; ok {
		return fmt.Errorf("meta srcpath:%s offset: %d size: %d cid: %s exist", cm.SrcPath, cm.SrcOffset, cm.Size, c.String())
	}
	ms.mappings[c] = cm
	ms.chunkRawSize[c] = rawSize
	return nil
}

func (ms *MappingService) updateMapping(c cid.Cid, offset uint64) error {
	ms.lk.Lock()
	defer ms.lk.Unlock()

	// Associating mapping information between the source data and the data in the CAR file based on the node CID.
	if _, ok := ms.mappings[c]; !ok {
		return fmt.Errorf("meta cid: %s is not exist", c.String())
	}

	// Calculating the starting position of a node within the CAR file.
	if rs, ok := ms.chunkRawSize[c]; ok {
		sum := rs + uint64(len(c.Bytes()))
		buf := make([]byte, 8)
		n := binary.PutUvarint(buf, sum)
		offset = offset - uint64(n)
	}
	ms.mappings[c].DstOffset = offset

	return nil
}

// Saving the cached mapping information to a file.
func (ms *MappingService) SaveMetaMappings(path string, name string) error {
	os.MkdirAll(path, 0o775)

	m := &types.Mapping{
		DataRoot: ms.dataRoot,
		Mappings: make([]*types.ChunkMapping, 0),
	}
	ms.lk.Lock()
	defer ms.lk.Unlock()
	for _, v := range ms.mappings {
		m.Mappings = append(m.Mappings, v)
	}

	sort.Slice(m.Mappings, func(i int, j int) bool {
		return m.Mappings[i].DstOffset < m.Mappings[j].DstOffset
	})

	metaPath := filepath.Join(path, name)
	return utils.WriteJson(metaPath, "\t", m)
}

// Loading mapping information from a file into the MappingService cache.
func (ms *MappingService) LoadMetaMappings(path string) error {
	var m types.Mapping
	err := utils.ReadJson(path, &m)
	if err != nil {
		return err
	}
	ms.dataRoot = m.DataRoot
	for _, v := range m.Mappings {
		ms.mappings[v.Cid] = v
	}
	return nil
}

// Verifying if the mapping information represents a continuous segment of data within the CAR file.
func (ms *MappingService) verifyMappingsContinuity(mappings []*types.ChunkMapping) error {
	var nextStart uint64
	for i, v := range mappings {
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

// Getting all mapping information from the MappingService cache.
func (ms *MappingService) GetAllChunkMappings() ([]*types.ChunkMapping, error) {
	var mappings []*types.ChunkMapping

	for _, v := range ms.mappings {
		mappings = append(mappings, v)
	}

	sort.Slice(mappings, func(i, j int) bool {
		return mappings[i].DstOffset < mappings[j].DstOffset
	})
	err := ms.verifyMappingsContinuity(mappings)
	return mappings, err
}

// Getting mapping information for data fragments associated with a given offset and size of car.
func (ms *MappingService) GetChunkMappings(dstOffset uint64, dstSize uint64) ([]*types.ChunkMapping, error) {
	chunkStart := dstOffset
	chunkEnd := dstOffset + dstSize
	var mappings []*types.ChunkMapping

	for _, v := range ms.mappings {
		start, end := v.ChunkRangeInCar()
		if chunkStart <= start && chunkEnd >= start && chunkEnd <= end ||
			chunkStart >= start && chunkStart <= end && chunkEnd >= end ||
			chunkStart <= start && chunkEnd >= end ||
			chunkStart >= start && chunkEnd <= end {
			mappings = append(mappings, v)
		}
	}

	sort.Slice(mappings, func(i, j int) bool {
		return mappings[i].DstOffset < mappings[j].DstOffset
	})

	err := ms.verifyMappingsContinuity(mappings)

	return mappings, err
}

// Generating a data fragment file of a CAR file based on contiguous mapping information
// and data obtained from the source file.
// For example, generating a 2MB data fragment for which a consistency proof needs to be challenged.
func (ms *MappingService) GenerateChunksFromMappings(path string, srcParent string, mappings []*types.ChunkMapping) error {
	cidBuilder, err := merkledag.PrefixForCidVersion(1)
	if err != nil {
		return err
	}
	// Target data fragment file.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	// Writing the header information of the CAR file first.
	pack.WriteCarHeader(file, ms.dataRoot)

	for _, m := range mappings {
		var err error
		var node ipld.Node
		if m.SrcPath != "" {
			// When SrcPath is not empty, data needs to be obtained from the source file to construct the current node.
			node, err = ms.GenerateNodeFromSource(path, srcParent, m, cidBuilder)
			if err != nil {
				return err
			}
		} else {
			// When SrcPath is empty, construct the node directly from the mapping information.
			node, err = ms.GenerateNodeWithoutData(m, cidBuilder)
			if err != nil {
				return err
			}
		}
		// If the constructed node does not match the node CID recorded in the mapping information, it indicates that the newly built node is incorrect.
		if node.Cid().String() != m.Cid.String() {
			return fmt.Errorf("The generated CID for the node is not consistent with the metadata record.")
		}

		// Writing a node to the CAR fragment file at a specified offset based on the mapping information.
		_, err = file.Seek(int64(m.DstOffset), 0)
		if err != nil {
			return err
		}
		pack.WriteCarBlock(file, node)
	}

	return nil
}

// Node construction without involving the source data.
func (ms *MappingService) GenerateNodeWithoutData(m *types.ChunkMapping, cidBuilder cid.Builder) (ipld.Node, error) {
	fsNode := helpers.NewFSNodeOverDag(m.NodeType, cidBuilder)
	for _, link := range m.Links {
		cm, ok := ms.mappings[link.Cid]
		if !ok {
			return nil, fmt.Errorf("cant find meta ,cid:%s", link.Cid.String())
		}
		var blockSize uint64
		if m.NodeType == pb.Data_File {
			blockSize = cm.Size
		} else {
			blockSize = cm.BlockSize
		}

		if err := fsNode.AddLinkChildToFsNode(link, blockSize); err != nil {
			return nil, err
		}
	}
	node, err := fsNode.Commit()
	if err != nil {
		return nil, err
	}
	return node, nil
}

// Node construction involving the source data.
func (ms *MappingService) GenerateNodeFromSource(path string, srcParent string, m *types.ChunkMapping, cidBuilder cid.Builder) (ipld.Node, error) {
	// Getting the source file path.
	srcPath := filepath.Join(srcParent, m.SrcPath)
	sfile, err := os.Open(srcPath)
	if err != nil {
		return nil, err
	}
	defer sfile.Close()
	data := make([]byte, m.Size)

	// Fetching smaller data fragments only from the source file at a specified offset.
	// This can be further adapted to work with cloud storage systems like AWS.
	if _, err := sfile.ReadAt(data, int64(m.SrcOffset)); err != nil {
		return nil, err
	}

	node, err := helpers.NewLeafNode(data, m.NodeType, cidBuilder, ms.opts.rawLeaves)
	if err != nil {
		return nil, err
	}

	node = helpers.ProcessFileStore(node, m.Size)
	if _, ok := ms.mappings[node.Cid()]; !ok {
		return nil, fmt.Errorf("generate new node from source failed:%s", err.Error())
	}

	return node, nil
}

// Access path for metadata.
func (ms *MappingService) MetaPath() string {
	return ms.opts.metaPath
}

// Access path for source data.
func (ms *MappingService) SourceParentPath() string {
	return ms.opts.sourceParentPath
}

// Use this function to generate data fragments of the CAR file at challenge points when creating challenge proofs.
func GetChallengeChunk(commCid cid.Cid, offset uint64, size uint64) ([]byte, error) {
	ms := MappingServiceInstance()
	metaPath := filepath.Join(ms.MetaPath(), commCid.String()+MAPPING_FILE_SUFFIX)
	if !utils.PathExists(metaPath) {
		return nil, fmt.Errorf("cant find meta file:%s", metaPath)
	}

	// Loading mapping files.
	ms.LoadMetaMappings(metaPath)

	// Getting mapping information for challenge points.
	mappings, err := ms.GetChunkMappings(offset, size)
	if err != nil {
		return nil, err
	}

	tempDir, err := os.MkdirTemp("", "")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tempDir)

	targetPath := filepath.Join(tempDir, commCid.String()+".car")

	// Generating temporary data fragment files.
	if err := ms.GenerateChunksFromMappings(targetPath, ms.SourceParentPath(), mappings); err != nil {
		return nil, err
	}

	file, err := os.Open(targetPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Get fragment file cache from a specific offset in the temporary file.
	buf := make([]byte, size)
	if _, err := file.ReadAt(buf, int64(offset)); err != nil {
		return nil, err
	}

	return buf, nil
}
