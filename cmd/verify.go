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
	ArgsUsage: "<randomness> <carSize> <dataSize> <cachePath>",
	Action:    verify,
}

// verify is a command to verify challenge proofs of merkle-tree.
func verify(c *cli.Context) error {
	if c.Args().Len() != 4 {
		return xerrors.Errorf("Args must be specified 4 nums!")
	}

	randomness, _ := strconv.ParseUint(c.Args().First(), 10, 64)
	carSize, _ := strconv.ParseUint(c.Args().Get(1), 10, 64)
	dataSize, _ := strconv.ParseUint(c.Args().Get(2), 10, 64)
	cachePath := c.Args().Get(3)

	bl, err := metaservice.Verify(randomness, carSize, dataSize, cachePath)
	if err != nil {
		return err
	}

	log.Info("\nverify: ", bl)
	return nil
}
