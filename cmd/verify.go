package main

import (
	metaservice "github.com/dataswap/go-metadata/service"
	"github.com/urfave/cli/v2"

	"golang.org/x/xerrors"
)

var verifyCmd = &cli.Command{
	Name:      "verify",
	Usage:     "verify challenge proofs of merkle-tree",
	ArgsUsage: "<cachePath>",
	Action:    verify,
}

// verify is a command to verify challenge proofs of merkle-tree.
func verify(c *cli.Context) error {
	if c.Args().Len() != 1 {
		return xerrors.Errorf("Args must be specified 1 nums!")
	}

	cachePath := c.Args().First()

	bl, err := metaservice.VerifyChallengeProof(cachePath)
	if err != nil {
		return err
	}

	log.Info("\nverify: ", bl)
	return nil
}
