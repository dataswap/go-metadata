package main

import (
	metaservice "github.com/dataswap/go-metadata/service"
	"github.com/urfave/cli/v2"

	"golang.org/x/xerrors"
)

var dumpCmd = &cli.Command{
	Name:      "dump",
	Usage:     "dump commp info",
	ArgsUsage: "<cachePath>",
	Action:    dump,
}

// dump is a command to dump commp info.
func dump(c *cli.Context) error {
	if c.Args().Len() != 1 {
		return xerrors.Errorf("Args must be specified 1 num!")
	}

	cachePath := c.Args().First()

	// rawCommP := [][]byte{
	// 	[]byte("aaaa"),
	// 	[]byte("bbbb"),
	// 	[]byte("cccc"),
	// 	[]byte("dddd"),
	// }

	// metaservice.SaveCommP(rawCommP[0], uint64(66), cachePath)
	// metaservice.SaveCommP(rawCommP[1], uint64(6677), cachePath)
	// metaservice.SaveCommP(rawCommP[2], uint64(6688), cachePath)
	// metaservice.SaveCommP(rawCommP[3], uint64(6699), cachePath)

	commP, carSize := metaservice.LoadSortCommp(cachePath)

	if commP == nil {
		log.Info("\nError: commP is nil")
	}
	log.Info("\ncommP: ", commP, "\ncarSize: ", carSize)
	return nil
}
