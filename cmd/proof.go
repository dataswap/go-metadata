package main

import (
	"strconv"

	metaservice "github.com/dataswap/go-metadata/service"
	"github.com/urfave/cli/v2"

	"golang.org/x/xerrors"
)

var proofCmd = &cli.Command{
	Name:      "proof",
	Usage:     "compute proof of merkle-tree",
	ArgsUsage: "<randomness> <cachePath>",
	Action:    proof,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "meta-path",
			Usage:    "The meta file",
			Required: true,
		},
		&cli.StringFlag{
			Name:     "source-parent-path",
			Usage:    "The source file",
			Required: true,
		},
		&cli.BoolFlag{
			Name:  "raw-leaves",
			Usage: "The raw leaves",
			Value: false,
		},
	},
}

// proof is a command to compute proof of commps.
func proof(c *cli.Context) error {
	if c.Args().Len() != 2 {
		return xerrors.Errorf("Args must be specified 2 nums!")
	}

	randomness, _ := strconv.ParseUint(c.Args().First(), 10, 64)
	cachePath := c.Args().Get(1)

	msrv := metaservice.MetaServiceInstance(
		metaservice.MetaPath(c.String("meta-path")),
		metaservice.SourceParentPath(c.String("source-parent-path")),
		metaservice.RawLeaves(c.Bool("raw-leaves")),
	)

	if err := msrv.LoadMeta(c.String("meta-path")); err != nil {
		return err
	}

	_, err := metaservice.Proof(randomness, cachePath)
	if err != nil {
		return err
	}

	return nil
}
