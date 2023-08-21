package main

import (
	metaservice "github.com/dataswap/go-metadata/service"
	"github.com/urfave/cli/v2"

	"golang.org/x/xerrors"
)

var tproofCmd = &cli.Command{
	Name:      "top-proof",
	Usage:     "compute top proof of commPs",
	ArgsUsage: "<cachePath>",
	Action:    tProof,
}

// tProof is a command to compute proof of commps.
func tProof(c *cli.Context) error {
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
