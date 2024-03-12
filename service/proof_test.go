package metaservice

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	"path"
	"testing"

	commcid "github.com/filecoin-project/go-fil-commcid"
	commp "github.com/filecoin-project/go-fil-commp-hashhash"
	"github.com/ipfs/go-cid"
	"github.com/ipld/go-car"
	"github.com/ipld/go-car/v2/blockstore"
	ipldprime "github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/node/basicnode"
	"github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"
)

func TestGenCommP(t *testing.T) {

	bs, err := blockstore.OpenReadOnly("../testdata/output/baga6ea4seaqopy46styyssotgxlat2vh3ksiukehesphcvoprskkq74o2yudmoi.car")
	if err != nil {
		t.Errorf("GenCommP OpenReadOnly err ")
	}

	cid, err := cid.Parse("bafybeiekw7iaz4zjgfq3gdcyh2zh77m3j5ns75w7lyu5nqq3bgoccjgzmq")
	if err != nil {
		t.Errorf("GenCommP Parse err")
	}

	cachePath := "../testdata/output"

	selector := allSelector()
	parentCtx := context.Background()
	sc := car.NewSelectiveCar(parentCtx, bs, []car.Dag{{Root: cid, Selector: selector}})

	cp := new(commp.Calc)
	sc.Write(cp)
	rawCommP1, pieceSize1, _ := cp.Digest()

	buf := bytes.Buffer{}
	sc.Write(&buf)

	rawCommP, pieceSize, err := GenCommP(buf, cachePath, 0)
	if err != nil {
		t.Errorf("GenCommP err")
	}

	if !bytes.Equal(rawCommP, rawCommP1) {
		t.Errorf("rawCommP != rawCommP1: %d, %d", rawCommP, rawCommP1)
	}

	if pieceSize != pieceSize1 {
		t.Errorf("pieceSize != pieceSize1: %d, %d", pieceSize, pieceSize1)
	}
}

func TestGenDatasetProof(t *testing.T) {

	saveCommpCache()
	cachePath := "../testdata/output"
	_, err := GenDatasetProof(cachePath)
	if err != nil {
		t.Errorf("Proof fail")
	}

	bl, _, err := VerifyDatasetProof(cachePath, 1)
	if !bl || err != nil {
		t.Errorf("Verify fail")
	}
}

func TestChallengeProof(t *testing.T) {

	saveCommpCache()
	randomness, _ := rand.Int(rand.Reader, big.NewInt(100))
	cachePath := "../testdata/output"

	MappingServiceInstance(
		MetaPath("../testdata/output/metas"),
		SourceParentPath("../testdata"),
	)

	_, err := GenChallengeProof(randomness.Uint64(), cachePath)
	if err != nil {
		t.Errorf("Proof fail")
	}

	bl, err := VerifyChallengeProof(cachePath)
	if err != nil || !bl {
		t.Errorf("VerifyChallengeProof fail")
	}
}

func allSelector() ipldprime.Node {
	ssb := builder.NewSelectorSpecBuilder(basicnode.Prototype.Any)
	return ssb.ExploreRecursive(selector.RecursionLimitNone(),
		ssb.ExploreAll(ssb.ExploreRecursiveEdge())).
		Node()
}

func saveCommpCache() {

	p := map[string]string{"../testdata/output/baga6ea4seaqopy46styyssotgxlat2vh3ksiukehesphcvoprskkq74o2yudmoi.car": "bafybeiekw7iaz4zjgfq3gdcyh2zh77m3j5ns75w7lyu5nqq3bgoccjgzmq",
		"../testdata/output/baga6ea4seaqkq2y6yhslmwrm4472d4qkzqubeki73z3qeei23e6bejuzjdxiygy.car": "bafybeicdsaojbbmf3dum3abtpdfvnm5vjan2yobzh74d22qxwklc64tzce"}

	for k, v := range p {
		bs, err := blockstore.OpenReadOnly(k)
		if err != nil {
			fmt.Println("GenCommP OpenReadOnly err ")
		}

		cid, err := cid.Parse(v)
		if err != nil {
			fmt.Println("GenCommP Parse err")
		}

		selector := allSelector()
		parentCtx := context.Background()
		sc := car.NewSelectiveCar(parentCtx, bs, []car.Dag{{Root: cid, Selector: selector}})

		buf := bytes.Buffer{}
		sc.Write(&buf)

		cachePath := "../testdata/output"

		rawCommP, _, err := GenCommP(buf, cachePath, 0)
		if err != nil {
			fmt.Println("GenCommP err")
		}
		SaveCommP(rawCommP, uint64(buf.Len()), cachePath)

		commCid, _ := commcid.DataCommitmentV1ToCID(rawCommP)
		err = os.Rename(path.Join(cachePath, hex.EncodeToString(rawCommP)+".cache"), path.Join(cachePath, commCid.String()+".cache"))
		if err != nil {
			fmt.Println("Rename err")
		}
	}
}
