package libs

import (
	"io"
	"path/filepath"

	chunker "github.com/ipfs/go-ipfs-chunker"
)

const UnixfsChunkSize uint64 = 1 << 20 //Deafault chunksize 2M

// Reading data while obtaining source data information, including the source data file path, offset, and size:
type SliceMeta struct {
	Path   string
	Offset uint64
	Size   uint64
}

// Defining the EnhancedSplitter interface as an extension of the Splitter interface,
// which reads data while also returning source data path, offset, and size
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

func NewSplitter(r io.Reader, size int64, srcPath string, parentPath string) (EnhancedSplitter, error) {
	path, err := filepath.Rel(filepath.Clean(parentPath), filepath.Clean(srcPath))
	if err != nil {
		return nil, err
	}
	spl := chunker.NewSizeSplitter(r, size)
	return &sliceSplitter{
		Splitter: spl,
		srcPath:  path,
		offset:   0,
	}, nil
}

func (ss *sliceSplitter) NextBytesWithMeta() ([]byte, *SliceMeta, error) {
	buf, err := ss.NextBytes()
	size := len(buf)

	//Constructing a source data SliceMeta
	m := &SliceMeta{
		Path:   ss.srcPath,
		Offset: ss.offset,
		Size:   uint64(size),
	}

	ss.offset += uint64(size)
	return buf, m, err
}

var _ EnhancedSplitter = &sliceSplitter{}
