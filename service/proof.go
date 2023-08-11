package metaservice

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"math/bits"
	"os"
	"path"

	sha256simd "github.com/minio/sha256-simd"
	"github.com/opentracing/opentracing-go/log"
	"golang.org/x/xerrors"

	mt "github.com/txaty/go-merkletree"
)

const (
	SOURCE_CHUNK_SIZE = 127
	SLAB_CHUNK_SIZE   = 128
	NODE_SIZE         = 32
	CHUNK_NODES_NUM   = 4

	// MaxLayers is the current maximum height of the rust-fil-proofs proving tree.
	MaxLayers = uint(31) // result of log2( 64 GiB / 32 )
	// MaxPieceSize is the current maximum size of the rust-fil-proofs proving tree.
	MaxPieceSize = uint64(1 << (MaxLayers + 5))
)

var (
	StackedNulPadding [MaxLayers][]byte
	SumChunkCount     uint64
	CommpHashConfig   = &mt.Config{
		HashFunc:           NewHashFunc,
		DisableLeafHashing: true,
		Mode:               mt.ModeTreeBuild,
	}
)

// DataBlock is a implementation of the DataBlock interface.
type DataBlock struct { // mt
	Data []byte
}

// Serialize returns the serialized form of the DataBlock.
func (t *DataBlock) Serialize() ([]byte, error) {
	return t.Data, nil
}

// SHA256 hash generate function for commp
func NewHashFunc(data []byte) ([]byte, error) {
	sha256Func := sha256simd.New()
	sha256Func.Write(data)
	rst := sha256Func.Sum(nil)
	rst[31] &= 0x3F
	return rst, nil
}

// SHA256 DataPadding function for commp
func DataPadding(inSlab []byte) []byte {

	chunkCount := len(inSlab) / SOURCE_CHUNK_SIZE
	SumChunkCount += uint64(chunkCount)
	outSlab := make([]byte, chunkCount*SLAB_CHUNK_SIZE)

	for j := 0; j < chunkCount; j++ {
		// Cycle over four(4) 31-byte groups, leaving 1 byte in between:
		// 31 + 1 + 31 + 1 + 31 + 1 + 31 = 127
		input := inSlab[j*SOURCE_CHUNK_SIZE : (j+1)*SOURCE_CHUNK_SIZE]
		expander := outSlab[j*SLAB_CHUNK_SIZE : (j+1)*SLAB_CHUNK_SIZE]
		inputPlus1, expanderPlus1 := input[1:], expander[1:]

		// First 31 bytes + 6 bits are taken as-is (trimmed later)
		// Note that copying them into the expansion buffer is mandatory:
		// we will be feeding it to the workers which reuse the bottom half
		// of the chunk for the result
		copy(expander[:], input[:32])

		// first 2-bit "shim" forced into the otherwise identical bitstream
		expander[31] &= 0x3F

		//  In: {{ C[7] C[6] }} X[7] X[6] X[5] X[4] X[3] X[2] X[1] X[0] Y[7] Y[6] Y[5] Y[4] Y[3] Y[2] Y[1] Y[0] Z[7] Z[6] Z[5]...
		// Out:                 X[5] X[4] X[3] X[2] X[1] X[0] C[7] C[6] Y[5] Y[4] Y[3] Y[2] Y[1] Y[0] X[7] X[6] Z[5] Z[4] Z[3]...
		for i := 31; i < 63; i++ {
			expanderPlus1[i] = inputPlus1[i]<<2 | input[i]>>6
		}

		// next 2-bit shim
		expander[63] &= 0x3F

		//  In: {{ C[7] C[6] C[5] C[4] }} X[7] X[6] X[5] X[4] X[3] X[2] X[1] X[0] Y[7] Y[6] Y[5] Y[4] Y[3] Y[2] Y[1] Y[0] Z[7] Z[6] Z[5]...
		// Out:                           X[3] X[2] X[1] X[0] C[7] C[6] C[5] C[4] Y[3] Y[2] Y[1] Y[0] X[7] X[6] X[5] X[4] Z[3] Z[2] Z[1]...
		for i := 63; i < 95; i++ {
			expanderPlus1[i] = inputPlus1[i]<<4 | input[i]>>4
		}

		// next 2-bit shim
		expander[95] &= 0x3F

		//  In: {{ C[7] C[6] C[5] C[4] C[3] C[2] }} X[7] X[6] X[5] X[4] X[3] X[2] X[1] X[0] Y[7] Y[6] Y[5] Y[4] Y[3] Y[2] Y[1] Y[0] Z[7] Z[6] Z[5]...
		// Out:                                     X[1] X[0] C[7] C[6] C[5] C[4] C[3] C[2] Y[1] Y[0] X[7] X[6] X[5] X[4] X[3] X[2] Z[1] Z[0] Y[7]...
		for i := 95; i < 126; i++ {
			expanderPlus1[i] = inputPlus1[i]<<6 | input[i]>>2
		}

		// the final 6 bit remainder is exactly the value of the last expanded byte
		expander[127] = input[126] >> 2
	}

	return outSlab
}

// initialize the nul padding stack
func initStackedNulPadding() {
	digest := sha256.New()
	StackedNulPadding[0] = make([]byte, sha256.Size)
	for i := uint(1); i < MaxLayers; i++ {
		digest.Reset()
		digest.Write(StackedNulPadding[i-1]) // yes, got to...
		digest.Write(StackedNulPadding[i-1]) // ...do it twice
		StackedNulPadding[i] = digest.Sum(make([]byte, 0, sha256.Size))
		StackedNulPadding[i][31] &= 0x3F
	}
}

// PadCommP is experimental, do not use it.
func PadCommP(sourceCommP []byte, sourcePaddedSize, targetPaddedSize uint64) ([]byte, error) {

	if len(sourceCommP) != 32 {
		return nil, xerrors.Errorf("provided commP must be exactly 32 bytes long, got %d bytes instead", len(sourceCommP))
	}
	if bits.OnesCount64(sourcePaddedSize) != 1 {
		return nil, xerrors.Errorf("source padded size %d is not a power of 2", sourcePaddedSize)
	}
	if bits.OnesCount64(targetPaddedSize) != 1 {
		return nil, xerrors.Errorf("target padded size %d is not a power of 2", targetPaddedSize)
	}
	if sourcePaddedSize > targetPaddedSize {
		return nil, xerrors.Errorf("source padded size %d larger than target padded size %d", sourcePaddedSize, targetPaddedSize)
	}
	if sourcePaddedSize < 128 {
		return nil, xerrors.Errorf("source padded size %d smaller than the minimum of 128 bytes", sourcePaddedSize)
	}
	if targetPaddedSize > MaxPieceSize {
		return nil, xerrors.Errorf("target padded size %d larger than Filecoin maximum of %d bytes", targetPaddedSize, MaxPieceSize)
	}

	// noop
	if sourcePaddedSize == targetPaddedSize {
		return sourceCommP, nil
	}

	out := make([]byte, 32)
	copy(out, sourceCommP)

	s := bits.TrailingZeros64(sourcePaddedSize)
	t := bits.TrailingZeros64(targetPaddedSize)

	sha256Func := sha256simd.New()
	for ; s < t; s++ {
		sha256Func.Reset()
		sha256Func.Write(out)
		sha256Func.Write(StackedNulPadding[s-5]) // account for 32byte chunks + off-by-one padding tower offset
		out = sha256Func.Sum(out[:0])
		out[31] &= 0x3F
	}

	return out, nil
}

func Digest(buf bytes.Buffer, cacheStart int, cacheLevels uint, cachePath string) ([]byte, uint64, error) {
	return GenCommP(buf, cacheStart, cacheLevels, cachePath)
}

func GenCommP(buf bytes.Buffer, cacheStart int, cacheLevels uint, cachePath string) ([]byte, uint64, error) {

	// padding stack
	initStackedNulPadding()

	srcLen := buf.Len()

	// Padding source data
	if mod := srcLen % SOURCE_CHUNK_SIZE; mod != 0 {
		// log.Info("total padlen: ", SOURCE_CHUNK_SIZE-mod, ", srcLen: ", srcLen)
		buf.Write(make([]byte, SOURCE_CHUNK_SIZE-mod))
		srcLen = buf.Len()
	}

	// Struce blocks from source data
	idx := 0
	blocks := make([]mt.DataBlock, srcLen*CHUNK_NODES_NUM/SOURCE_CHUNK_SIZE)
	for j := 0; j < srcLen/SOURCE_CHUNK_SIZE; j++ {
		nodes := DataPadding(buf.Bytes()[j*SOURCE_CHUNK_SIZE : (j+1)*SOURCE_CHUNK_SIZE])
		for b := 0; b < CHUNK_NODES_NUM; b++ {
			block := &DataBlock{
				Data: nodes[b*NODE_SIZE : (b+1)*NODE_SIZE],
			}
			blocks[idx] = block
			idx++
		}
	}

	tree, _ := mt.NewWithPadding(CommpHashConfig, blocks, StackedNulPadding)

	paddedPieceSize := SumChunkCount * SLAB_CHUNK_SIZE
	// hacky round-up-to-next-pow2
	if bits.OnesCount64(paddedPieceSize) != 1 {
		paddedPieceSize = 1 << uint(64-bits.LeadingZeros64(paddedPieceSize))
	}

	if cacheStart >= 0 {
		lc, err := mt.NewLevelCache(tree, cacheStart, int(cacheLevels))
		if err != nil {
			log.Error(err)
			return nil, 0, err
		}
		cPath := path.Join(cachePath, hex.EncodeToString(tree.Root)+".cache")
		os.MkdirAll(cachePath, 0o775)
		if err = lc.StoreToFile(cPath); err != nil {
			log.Error(err)
			return nil, 0, err
		}
	}

	return tree.Root, paddedPieceSize, nil
}

func GenProofFromCache(leaf mt.DataBlock, file string) (*mt.Proof, []byte, error) {
	lc, err := mt.NewLevelCacheFromFile(file)
	if err != nil {
		log.Error(err)
		return nil, nil, nil
	}

	return lc.Prove(leaf, CommpHashConfig)
}

func AppendProof(base *mt.Proof, sub mt.Proof) (*mt.Proof, error) {
	return mt.AppendProof(base, sub)
}

func Verify(leaf mt.DataBlock, proof *mt.Proof, root []byte) (bool, error) {
	return mt.Verify(leaf, proof, root, CommpHashConfig)
}
