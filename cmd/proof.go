package main

import (
	"strconv"

	metaservice "github.com/dataswap/go-metadata/service"
	"github.com/urfave/cli/v2"

	"golang.org/x/xerrors"
)

var proofCmd = &cli.Command{
	Name:  "proof",
	Usage: "compute proof of merkle-tree",
	Subcommands: []*cli.Command{
		challengeProofCmd,
		datasetProofCmd,
	},
}

var challengeProofCmd = &cli.Command{
	Name:      "chanllenge-proof",
	Usage:     "compute proof of merkle-tree",
	ArgsUsage: "<randomness> <cachePath>",
	Action:    challengeProof,
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

// challengeProof is a command to compute proof of commps.
func challengeProof(c *cli.Context) error {
	if c.Args().Len() != 2 {
		return xerrors.Errorf("Args must be specified 2 nums!")
	}

	randomness, _ := strconv.ParseUint(c.Args().First(), 10, 64)
	cachePath := c.Args().Get(1)

	metaservice.MappingServiceInstance(
		metaservice.MetaPath(c.String("meta-path")),
		metaservice.SourceParentPath(c.String("source-parent-path")),
		metaservice.RawLeaves(c.Bool("raw-leaves")),
	)

	_, err := metaservice.Proof(randomness, cachePath)
	if err != nil {
		return err
	}

	return nil
}

var datasetProofCmd = &cli.Command{
	Name:      "dataset-proof",
	Usage:     "compute dataset proof of commPs",
	ArgsUsage: "<cachePath>",
	Action:    datasetProof,
}

// datasetProof is a command to compute proof of commps.
func datasetProof(c *cli.Context) error {
	if c.Args().Len() != 1 {
		return xerrors.Errorf("Args must be specified 1 nums!")
	}

	cachePath := c.Args().First()

	_, err := metaservice.GenTopProof(cachePath)
	if err != nil {
		return err
	}

	bl, _, err := metaservice.VerifyTopProof(cachePath, 1)
	if !bl || err != nil {
		return err
	}

	return nil
}
