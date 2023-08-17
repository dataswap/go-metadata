package metaservice

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"fmt"
	"math/bits"
	"os"
	"path"
	"reflect"
	"sort"
	"sync"
	"syscall"

	commcid "github.com/filecoin-project/go-fil-commcid"
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

	CAR_CHALLENGES_RATE = float64(0.001)
	CAR_32GIB_SIZE      = uint64(1 << 35)
	CAR_2MIB_CHUNK_SIZE = uint64(1 << 21)

	CACHE_SUFFIX        = ".cache"
	CACHE_T_SUFFIX      = ".tcache"
	CACHE_PROOFS_SUFFIX = ".proofs"

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
		RunInParallel:      true,
	}

	Once sync.Once
)

type DatasetMerkletree struct {
	Root   []byte
	Leaves [][]byte
}

// DataBlock is a implementation of the DataBlock interface.
type DataBlock struct { // mt
	Data []byte
}

//### Merkle-tree hash functions

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

// ------------------------------------------------------------

//### internal functions

type FileLock struct {
	lockFile *os.File
}

func NewFileLock(filePath string) (*FileLock, error) {
	lockFilePath := filePath + ".lock"
	lockFile, err := os.OpenFile(lockFilePath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	return &FileLock{lockFile: lockFile}, nil
}

func (f *FileLock) Lock() error {
	return syscall.Flock(int(f.lockFile.Fd()), syscall.LOCK_EX)
}

func (f *FileLock) Unlock() error {
	return syscall.Flock(int(f.lockFile.Fd()), syscall.LOCK_UN)
}

func (f *FileLock) Close() {
	f.lockFile.Close()
}

func bufferToDataBlocks(buf bytes.Buffer) []mt.DataBlock {
	// padding stack
	Once.Do(initStackedNulPadding)

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

	return blocks
}

func bytesToDataBlock(bt []byte) mt.DataBlock {
	return &DataBlock{
		Data: bt[0:NODE_SIZE],
	}
}

func bytesToDataBlocks(bt [][]byte) []mt.DataBlock {
	blocks := make([]mt.DataBlock, len(bt)*NODE_SIZE)
	for i, data := range bt {
		blocks[i] = &DataBlock{
			Data: data[0:NODE_SIZE],
		}
	}

	return blocks
}

func createPath(filePath string, fileName string) string {
	os.MkdirAll(filePath, 0o775)
	return path.Join(filePath, fileName)
}

func storeToFile(data interface{}, filePath string) error {
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := gob.NewEncoder(file)
	if err = encoder.Encode(data); err != nil {
		return err
	}
	return nil
}

func loadFromFile(filePath string, target interface{}) error {
	readFile, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer readFile.Close()

	decoder := gob.NewDecoder(readFile)
	if err = decoder.Decode(target); err != nil {
		return err
	}

	return nil
}

func loadDeduplicateCommP(filePath string) ([][]byte, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	nodeNum := len(data) / NODE_SIZE
	commP := make([][]byte, nodeNum)
	uniqueMap := make(map[string]bool)

	for i := 0; i < nodeNum; i++ {
		if !uniqueMap[string(data[i*NODE_SIZE:(i+1)*NODE_SIZE])] {
			uniqueMap[string(data[i*NODE_SIZE:(i+1)*NODE_SIZE])] = true
			commP[i] = data[i*NODE_SIZE : (i+1)*NODE_SIZE]
		}
	}

	return commP, nil
}

func sortCommPSlices(byteSlices [][]byte) {
	sort.Slice(byteSlices, func(i, j int) bool {
		return string(byteSlices[i]) < string(byteSlices[j])
	})
}

// ------------------------------------------------------------

// Car leaf challenge count
func LeafChallengeCount(carSize uint64) uint32 {
	if carSize >= CAR_32GIB_SIZE {
		return 172
	} else {
		return 2
	}
}

// GenChallenges is generate the challenges car nodes
func GenChallenges(randomness uint64, carSize uint64, dataSize uint64) (map[uint64][]uint64, error) {
	carChallenges := make(map[uint64][]uint64)

	carChallengesCount := float64(dataSize%carSize) * CAR_CHALLENGES_RATE
	leafChallengeCount := LeafChallengeCount(carSize)

	for i := uint64(0); i < uint64(carChallengesCount); i++ {
		carIndex := GenCarChallenge(randomness, i, dataSize, carSize)
		for j := uint32(0); j < leafChallengeCount; j++ {
			carChallenges[carIndex] = append(carChallenges[carIndex], GenLeafChallenge(randomness, carIndex, j, carSize))
		}
	}

	return carChallenges, nil
}

func GenCarChallenge(randomness uint64, carChallengeIndex uint64, dataSize uint64, carSize uint64) uint64 {
	sha256Func := sha256simd.New()

	bytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(bytes, randomness)
	sha256Func.Write(bytes)

	bytes = bytes[:0]
	binary.LittleEndian.PutUint64(bytes, carChallengeIndex)
	sha256Func.Write(bytes)

	hash := sha256Func.Sum(nil)

	leaf_challenge := binary.LittleEndian.Uint64(hash[:8])
	return leaf_challenge % dataSize / carSize
}

func GenLeafChallenge(randomness uint64, carIndex uint64, leafChallengeIndex uint32, carSize uint64) uint64 {
	sha256Func := sha256simd.New()

	bytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(bytes, randomness)
	sha256Func.Write(bytes)

	bytes = bytes[:0]
	binary.LittleEndian.PutUint64(bytes, carIndex)
	sha256Func.Write(bytes)

	bytes = bytes[:0]
	binary.LittleEndian.PutUint32(bytes, leafChallengeIndex)
	sha256Func.Write(bytes)

	hash := sha256Func.Sum(nil)

	leaf_challenge := binary.LittleEndian.Uint64(hash[:8])
	return leaf_challenge % carSize / NODE_SIZE
}

func GenProof(buf bytes.Buffer, leaf mt.DataBlock) (*mt.Proof, []byte, error) {
	blocks := bufferToDataBlocks(buf)

	tree, err := mt.NewWithPadding(CommpHashConfig, blocks, StackedNulPadding)
	if err != nil {
		log.Error(err)
		return nil, nil, err
	}

	proof, err := tree.Proof(leaf)
	if err != nil {
		return nil, nil, err
	}

	return proof, tree.Root, nil
}

func GenProofFromCache(leaf mt.DataBlock, file string) (*mt.Proof, []byte, error) {
	lc, err := mt.NewLevelCacheFromFile(file)
	if err != nil {
		log.Error(err)
		return nil, nil, nil
	}

	return lc.Prove(leaf, CommpHashConfig)
}

// Append base and sub proof
func AppendProof(base *mt.Proof, sub mt.Proof) (*mt.Proof, error) {
	return mt.AppendProof(base, sub)
}

// ------------------------------------------------------------

// ### export functions
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

// SaveCommP append
func SaveCommP(rawCommP []byte, cachePath string) error {
	cPath := createPath(cachePath, "rawCommP"+CACHE_SUFFIX)
	file, err := os.OpenFile(cPath, os.O_APPEND|os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	lock, err := NewFileLock(cachePath)
	if err != nil {
		fmt.Println("Lock File Error:", err)
		return err
	}
	defer lock.Close()

	if err := lock.Lock(); err != nil {
		fmt.Println("Lock Error:", err)
		return err
	}
	defer lock.Unlock()

	_, err = file.Write(rawCommP)
	if err != nil {
		return err
	}

	return nil
}

// GenCommP is the commP generate
func GenCommP(buf bytes.Buffer, cacheStart int, cacheLevels uint, cachePath string) ([]byte, uint64, error) {

	blocks := bufferToDataBlocks(buf)
	tree, _ := mt.NewWithPadding(CommpHashConfig, blocks, StackedNulPadding)

	paddedPieceSize := SumChunkCount * SLAB_CHUNK_SIZE
	// hacky round-up-to-next-pow2
	if bits.OnesCount64(paddedPieceSize) != 1 {
		paddedPieceSize = 1 << uint(64-bits.LeadingZeros64(paddedPieceSize))
	}

	if cacheStart >= 0 {
		var lc *mt.LevelCache
		var err error
		if cacheLevels == 0 {
			lc, err = mt.NewLevelCache(tree, cacheStart, tree.Depth-cacheStart)
		} else {
			lc, err = mt.NewLevelCache(tree, cacheStart, int(cacheLevels))
		}

		if err != nil {
			log.Error(err)
			return nil, 0, err
		}
		cPath := createPath(cachePath, hex.EncodeToString(tree.Root)+CACHE_SUFFIX)
		if err = lc.StoreToFile(cPath); err != nil {
			log.Error(err)
			return nil, 0, err
		}
	}

	return tree.Root, paddedPieceSize, nil
}

// Generate commPs Merkle-Tree root to .tcache, proofs{rootHash, leafHashes[]}
// filePath: commPs[]
// cachePath: store to file path
func GenTopProof(filePath string, cachePath string) ([]byte, error) {
	commPs, err := loadDeduplicateCommP(filePath)
	if err != nil {
		log.Error(err)
		return nil, err
	}
	sortCommPSlices(commPs)

	Leaves := bytesToDataBlocks(commPs)
	tree, err := mt.New(CommpHashConfig, Leaves)
	if err != nil {
		log.Error(err)
		return nil, err
	}

	cache := DatasetMerkletree{Root: tree.Root, Leaves: tree.Leaves}

	cPath := createPath(cachePath, hex.EncodeToString(tree.Root)+CACHE_T_SUFFIX)

	err = storeToFile(cache, cPath)
	if err != nil {
		log.Error(err)
		return nil, err
	}

	return tree.Root, nil
}

func VerifyTopProof(cachePath string, randomness uint64) (bool, *mt.Proof, error) {
	cache := DatasetMerkletree{}
	if err := loadFromFile(cachePath, cache); err != nil {
		return false, nil, err
	}
	Leaves := bytesToDataBlocks(cache.Leaves)
	tree, err := mt.New(CommpHashConfig, Leaves)
	if err != nil {
		log.Error(err)
		return false, nil, err
	}
	if !reflect.DeepEqual(tree.Root, cache.Root) {
		proof, err := tree.Proof(Leaves[randomness%uint64(len(Leaves))])
		if err != nil {
			return false, nil, err
		}

		return false, proof, nil
	}

	return true, nil, nil
}

// Generate challenge nodes Proofs
func Proof(randomness uint64, carSize uint64, dataSize uint64, cachePath string, ms GetMetaServiceHandle) (*[]mt.Proof, error) {
	// 1. Generate challenge nodes
	carChallenges, err := GenChallenges(randomness, carSize, dataSize)
	if err != nil {
		return nil, err
	}

	commPs, err := loadDeduplicateCommP(cachePath)
	if err != nil {
		log.Error(err)
		return nil, err
	}
	sortCommPSlices(commPs)

	// 2. Get challenge chunk data
	proofs := []mt.Proof{}
	for carIndex, LeavesIndex := range carChallenges {
		for leafIndex := range LeavesIndex {

			commCid, err := commcid.DataCommitmentV1ToCID(commPs[carIndex])
			if err != nil {
				return nil, err
			}
			buf, err := GetChallengeChunk(commCid, uint64(leafIndex)/CAR_2MIB_CHUNK_SIZE+1, CAR_2MIB_CHUNK_SIZE, ms)
			if err != nil {
				return nil, err
			}
			// 3. Generate a car chunk proof
			leaf := bytesToDataBlock(buf[uint64(leafIndex)%CAR_2MIB_CHUNK_SIZE : uint64(leafIndex)%CAR_2MIB_CHUNK_SIZE+uint64(NODE_SIZE)])
			proof, root, err := GenProof(*bytes.NewBuffer(buf), leaf)
			if err != nil {
				return nil, err
			}
			// 4. Generate cache proofs
			cacheProof, _, err := GenProofFromCache(bytesToDataBlock(root), cachePath)
			if err != nil {
				return nil, err
			}
			// 5. Concat proofs
			proof, err = AppendProof(proof, *cacheProof)
			if err != nil {
				return nil, err
			}

			proofs = append(proofs, *proof)
		}
	}

	// 6. Store to cache file
	cPath := createPath(cachePath, "challenges"+CACHE_PROOFS_SUFFIX)
	storeToFile(proofs, cPath)

	return &proofs, nil
}

// Verify challenge nodes Proof
func Verify(randomness uint64, carSize uint64, dataSize uint64, cachePath string) (bool, error) {
	// 1. Generate challenge nodes
	carChallenges, err := GenChallenges(randomness, carSize, dataSize)
	if err != nil {
		return false, err
	}

	commPs, err := loadDeduplicateCommP(cachePath)
	if err != nil {
		log.Error(err)
		return false, err
	}
	sortCommPSlices(commPs)

	cPath := path.Join(cachePath, "challenges"+CACHE_PROOFS_SUFFIX)
	var proofs []mt.Proof
	err = loadFromFile(cPath, proofs)
	if err != nil {
		return false, err
	}
	if len(proofs) != len(carChallenges)*int(LeafChallengeCount(carSize)) {
		return false, errors.New("proofs or root size error")
	}

	// 2. Get challenge chunk data
	i := 0
	for _, LeavesIndex := range carChallenges {
		for leafIndex := range LeavesIndex {
			//commCid, err := commcid.DataCommitmentV1ToCID(commPs[carIndex])
			//if err != nil {
			//	return false, err
			//}
			// buf := GetChallengeChunk(carIndex, leafIndex/CAR_2MIB_CHUNK_SIZE+1)
			buf := bytes.Buffer{}
			root := make([]byte, 32)
			// 3. Generate a car chunk proof
			leaf := bytesToDataBlock(buf.Bytes()[uint64(leafIndex)%CAR_2MIB_CHUNK_SIZE : uint64(leafIndex)%CAR_2MIB_CHUNK_SIZE+uint64(NODE_SIZE)])
			rst, err := mt.Verify(leaf, &proofs[i], root, CommpHashConfig)
			if err != nil || !rst {
				return false, err
			}
			i++
		}
	}

	return true, nil
}
