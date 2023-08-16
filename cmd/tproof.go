package main

import (
	metaservice "github.com/dataswap/go-metadata/service"
	"github.com/urfave/cli/v2"

	"golang.org/x/xerrors"
)

var tproofCmd = &cli.Command{
	Name:      "proof",
	Usage:     "compute proof of commPs",
	ArgsUsage: "<commPsPath> <cachePath>",
	Action:    tProof,
}

// tProof is a command to compute proof of commps.
func tProof(c *cli.Context) error {
	if c.Args().Len() != 4 {
		return xerrors.Errorf("Args must be specified 4 nums!")
	}

	commPsPath := c.Args().First()
	cachePath := c.Args().Get(1)

	_, err := metaservice.GenTopProof(commPsPath, cachePath)
	if err != nil {
		return err
	}

	bl, _, err := metaservice.VerifyTopProof(cachePath, 1)
	if !bl || err != nil {
		return err
	}

	return nil
}
