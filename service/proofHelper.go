package metaservice

import (
	"bytes"
	"fmt"
	"math/bits"
	"strconv"

	"github.com/dataswap/go-metadata/utils"
	"golang.org/x/xerrors"

	mt "github.com/txaty/go-merkletree"
)

// DataBlock is a implementation of the DataBlock interface.
type DataBlock struct { // mt
	Data []byte
}

// DatasetMerkletree represents the structure of a dataset Merkle tree.
type DatasetMerkletree struct {
	Root   []byte
	Leaves [][]byte
}

// Proofs represents the challenge proofs data structure.
type Proofs struct {
	Leaves []string
	Proofs []mt.Proof
}

// ChallengeProofs represents the challenge proofs data structure.
type ChallengeProofs struct {
	RandomSeed uint64
	Leaves     []string
	Siblings   [][]string
	Paths      []string
}

// DatasetProof represents the data structure of dataset proofs.
type DatasetProof struct {
	Root       string
	LeafHashes []string
	LeafSizes  []uint64
}

// CarChallenge struct represents the challenge information of a car, including the car index and corresponding challenges.
type CarChallenge struct {
	CarIndex uint64
	Leaves   []uint64
}

// Padding DataBlock, commp leaf node use
// targetPaddedSize = 0 use default paddedSize
func NewPaddedDataBlocksFromBuffer(buf bytes.Buffer, targetPaddedSize uint64) ([]mt.DataBlock, uint64, error) {
	// padding stack
	Once.Do(initStackedNulPadding)

	srcLen := buf.Len()

	// Padding source data
	if mod := srcLen % SOURCE_CHUNK_SIZE; mod != 0 {
		// fmt.Println("total padlen: ", SOURCE_CHUNK_SIZE-mod, ", srcLen: ", srcLen)
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

	sourcePaddedSize := uint64((srcLen / SOURCE_CHUNK_SIZE) * SLAB_CHUNK_SIZE)
	if targetPaddedSize != 0 {
		blocks, err := NewPaddedDataBlocksFromDataBlocks(blocks, sourcePaddedSize, targetPaddedSize)
		if err != nil {
			return nil, 0, err
		}
		return blocks, targetPaddedSize, nil
	}

	return blocks, sourcePaddedSize, nil
}

// Padding DataBlock, commp leaf node use
// targetPaddedSize = 0 use default paddedSize
func NewPaddedDataBlocksFromDataBlocks(dataBlocks []mt.DataBlock, sourcePaddedSize, targetPaddedSize uint64) ([]mt.DataBlock, error) {
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
		return dataBlocks, nil
	}

	s := bits.TrailingZeros64(sourcePaddedSize)
	t := bits.TrailingZeros64(targetPaddedSize)

	for ; s < t; s++ {
		dataBlocks = append(dataBlocks, &DataBlock{Data: StackedNulPadding[s-5]})
	}

	return dataBlocks, nil
}

// No padding DataBlock
func NewDataBlockFromBytes(data []byte) mt.DataBlock {
	return &DataBlock{
		Data: data[0:NODE_SIZE],
	}
}

// No padding DataBlock
func NewDataBlocksFromBytes(bt [][]byte) []mt.DataBlock {
	blocks := make([]mt.DataBlock, len(bt))
	for i, data := range bt {
		blocks[i] = &DataBlock{
			Data: data[0:NODE_SIZE],
		}
	}

	return blocks
}

// Serialize returns the serialized form of the DataBlock.
func (t *DataBlock) Serialize() ([]byte, error) {
	return t.Data, nil
}

// NewDatasetProof creates a new DatasetProof instance based on the provided DatasetMerkletree.
func NewDatasetProof(proof DatasetMerkletree, leafSizes []uint64) *DatasetProof {
	leafHashes := make([]string, len(proof.Leaves))
	for i, leaf := range proof.Leaves {
		leafHashes[i] = utils.ConvertToHexPrefix(leaf)
	}

	return &DatasetProof{
		Root:       utils.ConvertToHexPrefix(proof.Root),
		LeafHashes: leafHashes,
		LeafSizes:  leafSizes,
	}
}

// NewDatasetProofFromFile creates a new DatasetProof instance from the provided file path.
func NewDatasetProofFromFile(filePath string) (*DatasetProof, error) {
	var datasetProof DatasetProof
	err := utils.ReadJson(filePath, &datasetProof)
	if err != nil {
		return nil, err
	}

	return &datasetProof, nil
}

// proof returns a DatasetMerkletree representing the proof data of the current DatasetProof.
func (d *DatasetProof) proof() DatasetMerkletree {
	root, _ := utils.ParseHexWithPrefix(d.Root)

	leaves := make([][]byte, len(d.LeafHashes))
	for i, hash := range d.LeafHashes {
		leaf, _ := utils.ParseHexWithPrefix(hash)
		leaves[i] = leaf
	}

	return DatasetMerkletree{
		Root:   root,
		Leaves: leaves,
	}
}

// save saves the current DatasetProof instance to the provided file path.
func (d *DatasetProof) save(filePath string) error {
	return utils.WriteJson(filePath, "\t", d)
}

// append the Proofs.
func (p *Proofs) append(leaf string, proof mt.Proof) *Proofs {
	p.Leaves = append(p.Leaves, leaf)
	p.Proofs = append(p.Proofs, proof)
	return p
}

// NewChallengeProofs creates a new ChallengeProofs instance from the provided randomness and proof map.
func NewChallengeProofs(randomness uint64, proofs Proofs) *ChallengeProofs {
	var challengeProofs ChallengeProofs
	challengeProofs.RandomSeed = randomness
	challengeProofs.Leaves = proofs.Leaves

	for _, value := range proofs.Proofs {

		siblings := make([]string, len(value.Siblings))
		for i, sibling := range value.Siblings {
			siblings[i] = utils.ConvertToHexPrefix(sibling)
		}

		challengeProofs.Siblings = append(challengeProofs.Siblings, siblings)
		challengeProofs.Paths = append(challengeProofs.Paths, fmt.Sprintf("0x%x", value.Path))
	}
	return &challengeProofs
}

// NewChallengeProofsFromFile creates a new ChallengeProofs instance by reading from the provided file path.
func NewChallengeProofsFromFile(filePath string) (*ChallengeProofs, error) {

	var challengeProofs ChallengeProofs
	err := utils.ReadJson(filePath, &challengeProofs)
	if err != nil {
		return nil, err
	}

	return &challengeProofs, nil
}

// save saves the ChallengeProofs instance to the provided file path.
func (c *ChallengeProofs) save(filePath string) error {

	return utils.WriteJson(filePath, "\t", c)
}

// proof returns a map of proof data for the ChallengeProofs instance.
func (c *ChallengeProofs) proof() Proofs {
	var proofs Proofs

	for i, leaf := range c.Leaves {

		siblings := make([][]byte, len(c.Siblings[i]))
		for j, sibling := range c.Siblings[i] {
			siblings[j], _ = utils.ParseHexWithPrefix(sibling)
		}

		path, _ := strconv.ParseUint(c.Paths[i], 0, 32)
		proofs.append(leaf, mt.Proof{
			Siblings: siblings,
			Path:     uint32(path),
		})
	}

	return proofs
}
