package libs

import (
	cid "github.com/ipfs/go-cid"
	chunker "github.com/ipfs/go-ipfs-chunker"
	ipld "github.com/ipfs/go-ipld-format"
	dag "github.com/ipfs/go-merkledag"
	"github.com/ipfs/go-unixfs/importer/helpers"
	pb "github.com/ipfs/go-unixfs/pb"
)

type HelperAction func(node ipld.Node, nodeType pb.Data_DataType)

func DefaultHelperAction(c cid.Cid, nodeType pb.Data_DataType) {}

type WrapDagBuilder struct {
	db  *helpers.DagBuilderHelper
	hcb HelperAction
}

// TODO: ????  考虑对DagServer封装，并实现chunk信息记录,WrapDagBuilder全部废除
func WrappedDagBuilder(params *helpers.DagBuilderParams, spl chunker.Splitter, hcb HelperAction) (*WrapDagBuilder, error) {
	db, err := params.New(spl)
	if err != nil {
		return nil, err
	}
	return &WrapDagBuilder{
		db:  db,
		hcb: hcb,
	}, nil
}

func (w *WrapDagBuilder) Done() bool {
	return w.db.Done()
}

func (w *WrapDagBuilder) Next() ([]byte, error) {
	return w.db.Next()
}

func (w *WrapDagBuilder) GetDagServ() ipld.DAGService {
	return w.db.GetDagServ()
}

func (w *WrapDagBuilder) GetCidBuilder() cid.Builder {
	return w.db.GetCidBuilder()
}

func (w *WrapDagBuilder) NewLeafNode(data []byte, fsNodeType pb.Data_DataType) (ipld.Node, error) {
	return w.db.NewLeafNode(data, fsNodeType)
}

func (w *WrapDagBuilder) FillNodeLayer(node *helpers.FSNodeOverDag) error {
	return w.db.FillNodeLayer(node)
}

func (w *WrapDagBuilder) NewLeafDataNode(fsNodeType pb.Data_DataType) (node ipld.Node, dataSize uint64, err error) {
	fileData, err := w.Next()
	if err != nil {
		return nil, 0, err
	}

	dataSize = uint64(len(fileData))

	// Create a new leaf node containing the file chunk data.
	node, err = w.NewLeafNode(fileData, fsNodeType)
	if err != nil {
		return nil, 0, err
	}

	w.hcb(node, fsNodeType)

	// Convert this leaf to a `FilestoreNode` if needed.
	node = w.ProcessFileStore(node, dataSize)

	return node, dataSize, nil

}

func (w *WrapDagBuilder) ProcessFileStore(node ipld.Node, dataSize uint64) ipld.Node {
	return w.db.ProcessFileStore(node, dataSize)
}

func (w *WrapDagBuilder) Add(node ipld.Node) error {
	return w.db.Add(node)
}

func (w *WrapDagBuilder) Maxlinks() int {
	return w.db.Maxlinks()
}

func (w *WrapDagBuilder) NewFSNodeOverDag(fsNodeType pb.Data_DataType) *helpers.FSNodeOverDag {
	return w.db.NewFSNodeOverDag(fsNodeType)
}

func (w *WrapDagBuilder) NewFSNFromDag(nd *dag.ProtoNode) (*helpers.FSNodeOverDag, error) {
	return w.db.NewFSNFromDag(nd)
}

var _ helpers.Helper = &WrapDagBuilder{}
