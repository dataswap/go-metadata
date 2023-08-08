package libs

import (
	"context"

	cid "github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	pb "github.com/ipfs/go-unixfs/pb"
)

type DagServiceAction func(node ipld.Node)

func DefaultDagServiceAction(c cid.Cid, nodeType pb.Data_DataType) {}

type WrapDagService struct {
	ds ipld.DAGService

	cb DagServiceAction
}

func WrappedDagService(dagService ipld.DAGService, cb DagServiceAction) (ipld.DAGService, error) {
	return &WrapDagService{
		ds: dagService,
		cb: cb,
	}, nil
}

func (wds *WrapDagService) Add(ctx context.Context, node ipld.Node) error {
	wds.cb(node)
	return wds.ds.Add(ctx, node)
}

func (wds *WrapDagService) AddMany(ctx context.Context, nodes []ipld.Node) error {
	for _, node := range nodes {
		wds.cb(node)
	}
	return wds.ds.AddMany(ctx, nodes)
}
func (wds *WrapDagService) Get(ctx context.Context, c cid.Cid) (ipld.Node, error) {
	return wds.ds.Get(ctx, c)
}

func (wds *WrapDagService) GetMany(ctx context.Context, cids []cid.Cid) <-chan *ipld.NodeOption {
	return wds.ds.GetMany(ctx, cids)
}

func (wds *WrapDagService) Remove(ctx context.Context, c cid.Cid) error {
	return wds.ds.Remove(ctx, c)
}

func (wds *WrapDagService) RemoveMany(ctx context.Context, cids []cid.Cid) error {
	return wds.ds.RemoveMany(ctx, cids)
}

var _ ipld.DAGService = &WrapDagService{}
