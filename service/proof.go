package metaservice

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math/bits"
	"os"
	"path"
	"reflect"
	"sort"
	"sync"

	"github.com/dataswap/go-metadata/utils"
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

	CAR_32GIB_SIZE      = uint64(1 << 35)
	CAR_2MIB_CHUNK_SIZE = uint64(SOURCE_CHUNK_SIZE * CAR_2MIB_NODE_NUM) // source data node = 127, no padding
	CAR_512B_CHUNK_SIZE = uint64(SOURCE_CHUNK_SIZE * CAR_512B_NODE_NUM) // source data node = 127, no padding

	CAR_2MIB_NODE_NUM = uint64(1 << 20 * 2 / SLAB_CHUNK_SIZE) // = 2MIB / SLAB_CHUNK_SIZE
	CAR_512B_NODE_NUM = uint64(1 << 9 / SLAB_CHUNK_SIZE)      // = 512B / SLAB_CHUNK_SIZE

	LEAF_CHALLENGE_MAX_COUNT = 172
	LEAF_CHALLENGE_MIN_COUNT = 2

	CAR_2MIB_CACHE_LAYER_START  = 16
	CAR_512B_CACHE_LAYER_START  = 4
	CACHE_SUFFIX                = ".cache"
	CACHE_DATASET_PROOF_PATH    = "dataset.proof"
	CACHE_CHALLENGE_PROOFS_PATH = "challenges.proofs"
	PROOFS_PATH                 = "proofs"

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

// SaveCommP append. struct format: map{commmp:carSize}
func SaveCommP(rawCommP []byte, carSize uint64, cachePath string) error {

	cPath := createPath(cachePath, "rawCommP"+CACHE_SUFFIX)

	lock, err := utils.NewFileLock(cachePath)
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
	commp, _ := loadCommP(cPath)
	if commp == nil { // first is nil
		commp = &map[string]uint64{}
	}

	(*commp)[string(rawCommP)] = carSize

	utils.WriteGob(commp, cPath)

	return nil
}

// GenCommP is the commP generate. targetPaddedSize = 0 is use default padded size
func GenCommP(buf bytes.Buffer, cachePath string, targetPaddedSize uint64) ([]byte, uint64, error) {

	blocks, paddedPieceSize, err := NewPaddedDataBlocksFromBuffer(buf, targetPaddedSize)
	if err != nil {
		return nil, 0, err
	}

	tree, _ := mt.NewWithPadding(CommpHashConfig, blocks, StackedNulPadding)

	// paddedPieceSize := SumChunkCount * SLAB_CHUNK_SIZE
	// hacky round-up-to-next-pow2
	if bits.OnesCount64(paddedPieceSize) != 1 {
		paddedPieceSize = 1 << uint(64-bits.LeadingZeros64(paddedPieceSize))
	}

	cacheStart := CarCacheLayerStart(uint64(buf.Len()))

	lc, err := mt.NewLevelCache(tree, cacheStart, tree.Depth-cacheStart)

	if err != nil {
		log.Error(err)
		return nil, 0, err
	}
	cPath := createPath(cachePath, hex.EncodeToString(tree.Root)+CACHE_SUFFIX)
	if err = lc.StoreToFile(cPath); err != nil {
		log.Error(err)
		return nil, 0, err
	}

	return tree.Root, paddedPieceSize, nil
}

// Generate commPs Merkle-Tree root to .tcache, proofs{rootHash, leafHashes[]}
// cachePath: store to file path
func GenDatasetProof(cachePath string) ([]byte, error) {

	commPs, sizes := LoadSortCommp(cachePath)
	Leaves := NewDataBlocksFromBytes(commPs)
	cache := DatasetMerkletree{}

	if err := errors.New("the number of leaves must be greater than 0"); len(Leaves) < 1 {
		log.Error(err)
		return nil, err
	} else if len(Leaves) == 1 {
		if leave, err := Leaves[0].Serialize(); err != nil {
			return nil, err
		} else {
			cache = DatasetMerkletree{Root: leave, Leaves: [][]byte{leave}}
		}

	} else {
		tree, err := mt.New(CommpHashConfig, Leaves)
		if err != nil {
			log.Error(err)
			return nil, err
		}

		cache = DatasetMerkletree{Root: tree.Root, Leaves: tree.Leaves}
	}
	cPath := createPath(cachePath, CACHE_DATASET_PROOF_PATH)
	err := NewDatasetProof(cache, sizes).save(cPath)
	if err != nil {
		log.Error(err)
		return nil, err
	}

	return cache.Root, nil

}

// Verify commPs Merkle-Tree proof
func VerifyDatasetProof(cachePath string, randomness uint64) (bool, *mt.Proof, error) {

	cPath := createPath(cachePath, CACHE_DATASET_PROOF_PATH)
	datasetProof, err := NewDatasetProofFromFile(cPath)
	if err != nil {
		return false, nil, err
	}
	cache := datasetProof.proof()
	Leaves := NewDataBlocksFromBytes(cache.Leaves)

	if err := errors.New("the number of leaves must be greater than 0"); len(Leaves) < 1 {
		log.Error(err)
		return false, nil, err

	} else if len(Leaves) == 1 {
		if bytes.Equal(cache.Root, cache.Leaves[0]) {
			return true, nil, nil
		} else {
			return false, nil, nil
		}

	} else {
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
}

// Generate challenge nodes Proofs
func GenChallengeProof(randomness uint64, cachePath string) (map[string]mt.Proof, error) {

	// 1. Generate challenge nodes
	commPs, carSize := LoadSortCommp(cachePath)
	carChallenges, err := GenChallenges(randomness, uint64(len(commPs)), carSize)
	if err != nil {
		return nil, err
	}

	// 2. Get challenge chunk data
	challengeProof := make(map[string]mt.Proof)
	for carIndex, LeavesIndex := range carChallenges {

		carChunkSize, carChunkNum := CarChunkParams(carSize[carIndex])
		for _, leafIndex := range LeavesIndex {

			commCid, err := commcid.DataCommitmentV1ToCID(commPs[carIndex])
			if err != nil {
				return nil, err
			}
			buf, err := GetChallengeChunk(commCid, uint64((leafIndex/carChunkSize)*carChunkSize), carChunkSize)
			if err != nil {
				return nil, err
			}

			// 3. Generate a car chunk proof
			blocks, _, err := NewPaddedDataBlocksFromBuffer(*bytes.NewBuffer(buf), 0)
			if err != nil {
				return nil, err
			}
			proof, root, err := GenProof(blocks, blocks[leafIndex%carChunkNum])
			if err != nil {
				return nil, err
			}

			// 4. Generate a car cache proof
			cPath := createPath(cachePath, commCid.String()+CACHE_SUFFIX)
			cacheProof, _, err := GenProofFromCache(NewDataBlockFromBytes(root), cPath)
			if err != nil {
				return nil, err
			}

			// 5. Concat proofs
			proof, err = AppendProof(proof, *cacheProof)
			if err != nil {
				return nil, err
			}

			if proof == nil {
				return nil, errors.New("proof is nil")
			}
			leaf, err := blocks[leafIndex%carChunkNum].Serialize()
			if err != nil {
				return nil, err
			}
			challengeProof[utils.ConvertToHexPrefix(leaf)] = *proof
		}
	}

	// 6. Store to cache file
	cPath := createPath(cachePath, CACHE_CHALLENGE_PROOFS_PATH)
	NewChallengeProofs(randomness, challengeProof).save(cPath)

	return challengeProof, nil
}

// Verify challenge nodes Proof
func VerifyChallengeProof(cachePath string) (bool, error) {

	// 1. Load proofs
	cPath := path.Join(cachePath, CACHE_CHALLENGE_PROOFS_PATH)
	challengeProofs, err := NewChallengeProofsFromFile(cPath)
	if err != nil {
		return false, err
	}

	proofs := challengeProofs.proof()

	// 2. Generate challenge nodes
	commPs, carSize := LoadSortCommp(cachePath)
	if commPs == nil {
		return false, errors.New("commPs is nil")
	}
	carChallenges, err := GenChallenges(challengeProofs.RandomSeed, uint64(len(commPs)), carSize)
	if err != nil {
		return false, err
	}

	// 3. Verify proofs
	var idx []uint64
	i := 0
	for carIndex, LeavesIndex := range carChallenges {
		for range LeavesIndex {
			idx = append(idx, carIndex)
		}
	}

	for _leaf, proof := range proofs {
		leaf, _ := utils.ParseHexWithPrefix(_leaf)
		rst, err := mt.Verify(&DataBlock{Data: leaf}, &proof, commPs[idx[i]], CommpHashConfig)
		if err != nil || !rst {
			return false, err
		}
		i++
	}

	return true, nil
}

//### public functions

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

// LoadSortCommp loads and sorts the CommP values from the cache file.
// It takes the cache file path as input.
// It returns a slice of CommP values and a slice of their corresponding indices.
func LoadSortCommp(cachePath string) ([][]byte, []uint64) {
	cPath := createPath(cachePath, "rawCommP"+CACHE_SUFFIX)
	c, err := loadCommP(cPath)
	if err != nil {
		fmt.Println("loadCommP err: ", err)
		return nil, nil
	}
	return sortCommPSlices(*c)
}

// Car leaf challenge count.
func LeafChallengeCount(carSize uint64) uint32 {
	if carSize >= CAR_32GIB_SIZE {
		return LEAF_CHALLENGE_MAX_COUNT
	} else {
		return LEAF_CHALLENGE_MIN_COUNT
	}
}

// Car challenge count
func CarChallengeCount(carNum uint64) uint64 {
	if carNum < 1000 {
		return 1
	} else {
		return carNum/1000 + 1
	}
}

// CarChunkParams returns the chunk size and node number for a given CAR size.
// It takes the size of the CAR as input.
// It returns the chunk size and the number of nodes.
func CarChunkParams(carSize uint64) (uint64, uint64) {
	if carSize < CAR_2MIB_CHUNK_SIZE {
		return CAR_512B_CHUNK_SIZE, CAR_512B_NODE_NUM
	} else {
		return CAR_2MIB_CHUNK_SIZE, CAR_2MIB_NODE_NUM
	}
}

// CarCacheLayerStart returns the start index of the cache layer for a given CAR size.
// It takes the size of the CAR as input.
// It returns the start index of the cache layer.
func CarCacheLayerStart(carSize uint64) int {
	if carSize < CAR_2MIB_CHUNK_SIZE {
		return CAR_512B_CACHE_LAYER_START
	} else {
		return CAR_2MIB_CACHE_LAYER_START
	}
}

// GenChallenges is generate the challenges car nodes
func GenChallenges(randomness uint64, carNum uint64, carSize []uint64) (map[uint64][]uint64, error) {
	carChallenges := make(map[uint64][]uint64)

	carChallengesCount := CarChallengeCount(carNum)

	for i := uint64(0); i < carChallengesCount; i++ {
		carIndex := GenCarChallenge(randomness, i, carChallengesCount)
		leafChallengeCount := LeafChallengeCount(carSize[carIndex])
		for j := uint32(0); j < leafChallengeCount; j++ {
			carChallenges[carIndex] = append(carChallenges[carIndex], GenLeafChallenge(randomness, carIndex, j, carSize[carIndex]))
		}
	}

	return carChallenges, nil
}

// GenCarChallenge generates a car challenge index using randomness, the car challenge index, and the total number of car challenges.
// It returns the generated car challenge index.
func GenCarChallenge(randomness uint64, carChallengeIndex uint64, carChallengesCount uint64) uint64 {
	sha256Func := sha256simd.New()

	bytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(bytes[:8], randomness)
	sha256Func.Write(bytes)

	binary.LittleEndian.PutUint64(bytes[:8], carChallengeIndex)
	sha256Func.Write(bytes)

	hash := sha256Func.Sum(nil)

	carChallenge := binary.LittleEndian.Uint64(hash[:8])
	return carChallenge % carChallengesCount
}

// GenLeafChallenge generates a leaf challenge index using randomness, the car index, the leaf challenge index, and the size of the car.
// It returns the generated leaf challenge index.
func GenLeafChallenge(randomness uint64, carIndex uint64, leafChallengeIndex uint32, carSize uint64) uint64 {
	sha256Func := sha256simd.New()

	bytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(bytes[:8], randomness)
	sha256Func.Write(bytes)

	binary.LittleEndian.PutUint64(bytes[:8], carIndex)
	sha256Func.Write(bytes)

	bytes = make([]byte, 4)
	binary.LittleEndian.PutUint32(bytes[:4], leafChallengeIndex)
	sha256Func.Write(bytes)

	hash := sha256Func.Sum(nil)

	leaf_challenge := binary.LittleEndian.Uint64(hash[:8])
	return leaf_challenge % carSize
}

// GenProof generates a Merkle tree proof for the specified leaf block.
// It takes a slice of data blocks and the leaf block as input.
// It returns the proof, the root hash of the Merkle tree, and any error encountered.
func GenProof(blocks []mt.DataBlock, leaf mt.DataBlock) (*mt.Proof, []byte, error) {

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

// GenProofFromCache generates a Merkle tree proof for the specified leaf block using a level cache.
// It takes the leaf block and the cache file path as input.
// It returns the proof, the root hash of the Merkle tree, and any error encountered.
func GenProofFromCache(leaf mt.DataBlock, file string) (*mt.Proof, []byte, error) {
	lc, err := mt.NewLevelCacheFromFile(file)
	if err != nil {
		fmt.Println("NewLevelCacheFromFile error: ", err)
		return nil, nil, err
	}

	return lc.Prove(leaf, CommpHashConfig)
}

// Append base and sub proof
func AppendProof(base *mt.Proof, sub mt.Proof) (*mt.Proof, error) {
	if base == nil {
		return nil, errors.New("AppendProof base proof is nil")
	}
	return mt.AppendProof(base, sub)
}

//### internal functions

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

// createPath creates a directory path and returns the full file path by joining the directory path with the file name.
// It takes the directory path and the file name as input.
// It returns the full file path.
func createPath(filePath string, fileName string) string {
	os.MkdirAll(filePath, 0o775)
	return path.Join(filePath, fileName)
}

// loadCommP loads CommP values from a cache file.
// It takes the cache file path as input.
// It returns a map containing CommP values and an error if any.
func loadCommP(cachePath string) (*map[string]uint64, error) {
	commp := map[string]uint64{}
	if err := utils.ReadGob(cachePath, &commp); err != nil {
		return nil, err
	}
	return &commp, nil
}

// sortCommPSlices sorts CommP slices and returns them along with their corresponding sizes.
// It takes a map containing CommP values as input.
// It returns slices of CommP values and their corresponding sizes.
func sortCommPSlices(c map[string]uint64) ([][]byte, []uint64) {

	commp := make([][]byte, 0, len(c))
	for k := range c {
		commp = append(commp, []byte(k))
	}

	sort.Slice(commp, func(i, j int) bool {
		return bytes.Compare(commp[i], commp[j]) < 0
	})

	size := make([]uint64, 0, len(commp))
	for _, v := range commp {
		size = append(size, c[string(v)])
	}

	return commp, size
}
