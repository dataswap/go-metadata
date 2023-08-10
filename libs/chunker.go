package libs

import (
	"io"

	chunker "github.com/ipfs/go-ipfs-chunker"
)

const UnixfsChunkSize uint64 = 1 << 20 //Deafault chunksize 2M

type SliceMeta struct {
	Path   string
	Offset uint64
	Size   uint64
}

type EnhancedSplitter interface {
	chunker.Splitter
	NextBytesWithMeta() ([]byte, *SliceMeta, error)
}

type sliceSplitter struct {
	chunker.Splitter
	// source data's path
	srcPath string
	offset  uint64
}

func NewSplitter(r io.Reader, size int64, srcPath string) EnhancedSplitter {
	spl := chunker.NewSizeSplitter(r, size)
	return &sliceSplitter{
		Splitter: spl,
		offset:   0,
	}
}

func (ss *sliceSplitter) NextBytesWithMeta() ([]byte, *SliceMeta, error) {
	buf, err := ss.NextBytes()
	size := len(buf)
	m := &SliceMeta{
		Path:   ss.srcPath,
		Offset: ss.offset,
		Size:   uint64(size),
	}
	ss.offset += uint64(size)
	return buf, m, err
}

var _ EnhancedSplitter = &sliceSplitter{}
