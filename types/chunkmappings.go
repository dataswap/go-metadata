package types

import (
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	pb "github.com/ipfs/go-unixfs/pb"
)

// chunk meta mapping
type ChunkMapping struct {
	SrcPath   string           `json:"srcpath"`   // the path of chunk's source data
	SrcOffset uint64           `json:"srcoffset"` // the offset of chunk data in source data
	Size      uint64           `json:"size"`      // chunk data size
	DstOffset uint64           `json:"dstoffset"` // the offset of chunk in car
	ChunkSize uint64           `json:"chunksize"`
	BlockSize uint64           `json:"blocksize"`
	NodeType  pb.Data_DataType `json:"nodetype"`
	Cid       cid.Cid          `json:"cid"` // node cid
	Links     []*ipld.Link     `json:links` // chunks of node
}

func (cm *ChunkMapping) ChunkRangeInCar() (uint64, uint64) {
	return cm.DstOffset, cm.DstOffset + cm.ChunkSize
}

func (cm *ChunkMapping) ChunkRanageInSource() (string, uint64, uint64) {
	return cm.SrcPath, cm.SrcOffset, cm.SrcOffset + cm.Size
}

// source info
type SrcData struct {
	Path   string
	Offset uint64
	Size   uint64
}

type Mapping struct {
	DataRoot cid.Cid         `json:"dagroot"`
	Mappings []*ChunkMapping `json:"mappings"`
}
