package libs

import (
	"io"

	"github.com/ipfs/go-cid"
)

const DefaultCidSize = 38

type WriteAfterAction func(path string, cid cid.Cid, count int, offset uint64)

type WriteBeforeAction func([]byte, io.Writer) ([]byte, error)

func DefaultWriteAfterAction(path string, cid cid.Cid, count int, offset uint64) {}

func DefaultWriteBeforeAction(buf []byte, w io.Writer) ([]byte, error) { return buf, nil }

type WrapWriter struct {
	io.Writer
	path   string
	offset uint64
	count  int
	after  WriteAfterAction
	before WriteBeforeAction
}

func (bc *WrapWriter) Write(p []byte) (int, error) {
	buf, err := bc.before(p, bc.Writer)
	if err != nil {
		return 0, err
	}

	n, err := bc.Writer.Write(buf)
	if err == nil {
		size := len(p)
		bc.count = size
		if size == DefaultCidSize {
			c := cid.Undef
			if c, err = cid.Parse(p); err != nil {
				bc.after(bc.path, c, bc.count, bc.offset)
			}
		}
		bc.offset += uint64(size)
		return n, nil
	}

	return n, err
}

func WrappedWriter(w io.Writer, path string, acb WriteAfterAction, bcb WriteBeforeAction) io.Writer {
	wrapped := WrapWriter{
		Writer: w,
		path:   path,
		after:  acb,
		before: bcb,
	}
	return &wrapped
}
