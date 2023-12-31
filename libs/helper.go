package libs

import (
	"context"
	"io"
	"sync"

	cid "github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-unixfs/importer/helpers"
	pb "github.com/ipfs/go-unixfs/pb"
)

// Helper callback function, when Helper calls Add to add a node to DAGService,
// it passes the mapping information of the node to MappingService.
type HelperAction func(node ipld.Node, srcPath string, offset uint64, size uint64)

func DefaultHelperAction(node ipld.Node, srcPath string, offset uint64, size uint64) {
}

type WrapDagBuilder struct {
	helpers.Helper
	spl   EnhancedSplitter
	hcb   HelperAction
	dserv ipld.DAGService
	metas map[cid.Cid]*SliceMeta

	recvdErr error
	nextData []byte // the next item to return.
	nextMeta *SliceMeta

	lk sync.RWMutex
}

func WrappedDagBuilder(params *helpers.DagBuilderParams, spl EnhancedSplitter, hcb HelperAction) (*WrapDagBuilder, error) {
	db, err := params.New(spl)
	if err != nil {
		return nil, err
	}
	return &WrapDagBuilder{
		Helper: db,
		spl:    spl,
		dserv:  params.Dagserv,
		hcb:    hcb,
		metas:  make(map[cid.Cid]*SliceMeta, 0),
	}, nil
}

// Rewrite 'NewLeafDataNode' to cache the 'SliceMeta' information retrieved from the 'EnhancedSplitter'
func (w *WrapDagBuilder) NewLeafDataNode(fsNodeType pb.Data_DataType) (node ipld.Node, dataSize uint64, err error) {
	fileData, meta, err := w.next()
	if err != nil {
		return nil, 0, err
	}

	dataSize = uint64(len(fileData))

	// Create a new leaf node containing the file chunk data.
	node, err = w.NewLeafNode(fileData, fsNodeType)
	if err != nil {
		return nil, 0, err
	}

	// Convert this leaf to a `FilestoreNode` if needed.
	node = w.ProcessFileStore(node, dataSize)

	w.lk.Lock()
	defer w.lk.Unlock()

	// cache the 'SliceMeta' information retrieved from the 'EnhancedSplitter'
	w.metas[node.Cid()] = meta

	return node, dataSize, nil
}

// Rewrite the 'Add' method to invoke a callback function to pass back the mapping information of the node.
func (w *WrapDagBuilder) Add(node ipld.Node) error {
	w.lk.RLock()
	defer w.lk.RUnlock()
	if meta, ok := w.metas[node.Cid()]; ok {
		w.hcb(node, meta.Path, meta.Offset, meta.Size)
	}

	return w.dserv.Add(context.TODO(), node)
}

// Reimplement the 'prepareNext' function to make the helper retrieve data from the 'EnhancedSplitter' interface.
func (w *WrapDagBuilder) prepareNext() {
	// if we already have data waiting to be consumed, we're ready
	if w.nextData != nil || w.recvdErr != nil {
		return
	}

	w.nextData, w.nextMeta, w.recvdErr = w.spl.NextBytesWithMeta()
	if w.recvdErr == io.EOF {
		w.recvdErr = nil
	}
}

// Rewrite the 'Done' function to make the helper retrieve data from the 'EnhancedSplitter' interface.
func (w *WrapDagBuilder) Done() bool {
	// ensure we have an accurate perspective on data
	// as `done` this may be called before `next`.
	w.prepareNext() // idempotent
	if w.recvdErr != nil {
		return false
	}
	return w.nextData == nil
}

// Rewrite the 'Next' function to make the helper retrieve data from the 'EnhancedSplitter' interface.
func (w *WrapDagBuilder) Next() ([]byte, error) {
	w.prepareNext() // idempotent
	d := w.nextData
	w.nextData = nil // signal we've consumed it
	if w.recvdErr != nil {
		return nil, w.recvdErr
	}
	return d, nil
}

func (w *WrapDagBuilder) next() ([]byte, *SliceMeta, error) {
	buf, err := w.Next()
	return buf, w.nextMeta, err
}

var _ helpers.Helper = &WrapDagBuilder{}
