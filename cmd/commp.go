package main

import (
	"bytes"

	metaservice "github.com/dataswap/go-metadata/service"
	commcid "github.com/filecoin-project/go-fil-commcid"
	"github.com/ipfs/go-cid"
	"github.com/ipld/go-car"
	"github.com/urfave/cli/v2"

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

// commpCar is a command to output the commp cid in a car.
func commpCar(c *cli.Context) error {
	if c.Args().Len() != 2 {
		return xerrors.Errorf("CarPath and CarRoot must be specified!")
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
	rawCommP, pieceSize, err := metaservice.GenCommP(buf)
	if err != nil {
		return err
	}
	commCid, _ := commcid.DataCommitmentV1ToCID(rawCommP)

	log.Info("\nCommP Cid: ", commCid.String(), "\npieceSize: ", pieceSize)

	return nil
}

func allSelector() ipldprime.Node {
	ssb := builder.NewSelectorSpecBuilder(basicnode.Prototype.Any)
	return ssb.ExploreRecursive(selector.RecursionLimitNone(),
		ssb.ExploreAll(ssb.ExploreRecursiveEdge())).
		Node()
}
