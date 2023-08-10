package main

import (
	"bytes"

	sha256simd "github.com/minio/sha256-simd"

	commcid "github.com/filecoin-project/go-fil-commcid"
	"github.com/ipfs/go-cid"
	"github.com/ipld/go-car"
	"github.com/urfave/cli/v2"

	mt "github.com/txaty/go-merkletree"

	"github.com/ipld/go-car/v2/blockstore"
	ipldprime "github.com/ipld/go-ipld-prime"
	basicnode "github.com/ipld/go-ipld-prime/node/basic"
	"github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"
	"golang.org/x/xerrors"
)

var commpCmd = &cli.Command{
	Name:      "commp",
	Usage:     "compute commp CID(PieceCID)",
	ArgsUsage: "<inputCarPath> <inputCarRoot>",
	Action:    commpCar,
}

const (
	SOURCE_CHUNK_SIZE = 127
	SLAB_CHUNK_SIZE   = 128
	NODE_SIZE         = 32
	CHUNK_NODES_NUM   = 4
)

// DataBlock is a implementation of the DataBlock interface.
type DataBlock struct { // mt
	Data []byte
}

// Serialize returns the serialized form of the DataBlock.
func (t *DataBlock) Serialize() ([]byte, error) {
	return t.Data, nil
}

// Commp hash generate function
func NewHashFunc(data []byte) ([]byte, error) {
	sha256Func := sha256simd.New()
	sha256Func.Write(data)
	rst := sha256Func.Sum(nil)
	rst[31] &= 0x3F
	return rst, nil
}

// Commp DataPadding function
func DataPadding(inSlab []byte) []byte {

	quadsCount := len(inSlab) / SOURCE_CHUNK_SIZE
	outSlab := make([]byte, quadsCount*SLAB_CHUNK_SIZE)

	for j := 0; j < quadsCount; j++ {
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

// commpCar is a command to output the commp cid in a car.
func commpCar(c *cli.Context) error {
	if c.Args().Len() != 2 {
		return xerrors.Errorf("a car location must be specified")
	}

	bs, err := blockstore.OpenReadOnly(c.Args().First())
	if err != nil {
		return err
	}

	cid, err := cid.Parse(c.Args().Get(1))
	if err != nil {
		return err
	}

	selector := allSelector()
	sc := car.NewSelectiveCar(c.Context, bs, []car.Dag{{Root: cid, Selector: selector}})

	buf := bytes.Buffer{}
	sc.Write(&buf)

	count := buf.Len()

	if mod := count % SOURCE_CHUNK_SIZE; mod != 0 {
		// log.Info("total padlen: ", SOURCE_CHUNK_SIZE-mod, ", count: ", count)
		buf.Write(make([]byte, SOURCE_CHUNK_SIZE-mod))
		count = buf.Len()
	}

	idx := 0
	blocks := make([]mt.DataBlock, count*CHUNK_NODES_NUM/SOURCE_CHUNK_SIZE)
	for j := 0; j < count/SOURCE_CHUNK_SIZE; j++ {
		nodes := DataPadding(buf.Bytes()[j*SOURCE_CHUNK_SIZE : (j+1)*SOURCE_CHUNK_SIZE])
		for b := 0; b < CHUNK_NODES_NUM; b++ {
			block := &DataBlock{
				Data: nodes[b*NODE_SIZE : (b+1)*NODE_SIZE],
			}
			blocks[idx] = block
			idx++
		}
	}

	config := &mt.Config{
		HashFunc:           NewHashFunc,
		DisableLeafHashing: true,
		Mode:               mt.ModeProofGenAndTreeBuild,
	}
	tree, _ := mt.New(config, blocks)

	// Fetch the root hash of the tree
	rawCommP := tree.Root
	commCid, _ := commcid.DataCommitmentV1ToCID(rawCommP)

	log.Info("\nCommP Cid: ", commCid.String())

	return nil
}

func allSelector() ipldprime.Node {
	ssb := builder.NewSelectorSpecBuilder(basicnode.Prototype.Any)
	return ssb.ExploreRecursive(selector.RecursionLimitNone(),
		ssb.ExploreAll(ssb.ExploreRecursiveEdge())).
		Node()
}
