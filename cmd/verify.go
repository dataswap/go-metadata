package main

import (
	"strconv"

	metaservice "github.com/dataswap/go-metadata/service"
	"github.com/urfave/cli/v2"

	"golang.org/x/xerrors"
)

var verifyCmd = &cli.Command{
	Name:      "verify",
	Usage:     "verify challenge proofs of merkle-tree",
	ArgsUsage: "<randomness> <cachePath>",
	Action:    verify,
}

// verify is a command to verify challenge proofs of merkle-tree.
func verify(c *cli.Context) error {
	if c.Args().Len() != 2 {
		return xerrors.Errorf("Args must be specified 2 nums!")
	}

	randomness, _ := strconv.ParseUint(c.Args().First(), 10, 64)
	cachePath := c.Args().Get(1)

	bl, err := metaservice.Verify(randomness, cachePath)
	if err != nil {
		return err
	}

	log.Info("\nverify: ", bl)
	return nil
}
