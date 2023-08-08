package types

import (
	"github.com/ipfs/go-cid"
	pb "github.com/ipfs/go-unixfs/pb"
	"github.com/ipld/go-ipld-prime"
)

// chunk meta
type ChunkMeta struct {
	SrcPath   string           `json:"srcpath"`   // the path of chunk's source data
	SrcOffset uint64           `json:"srcoffset"` // the offset of chunk data in source data
	Size      uint32           `json:"size"`      // chunk data size
	DstPath   string           `json:"dstpath"`   // the car path
	DstOffset uint64           `json:"dstoffset"` // the offset of chunk in car
	NodeType  pb.Data_DataType `json:"nodetype"`
	Cid       cid.Cid          `json:"cid"` // node cid
	Links     []*ipld.Link     `json:links` // chunks of node
}

func (cm *ChunkMeta) GetDstRange(c cid.Cid) (uint64, uint64, error) {
	var start, end uint64
	//TODO: get the range of chunk in car
	return start, end, nil
}

// source info
type SrcData struct {
	Path   string
	Offset uint64
	Size   uint32
}
