package main

import (
	metaservice "github.com/dataswap/go-metadata/service"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

var createChunksCmd = &cli.Command{
	Name:      "create-chunks",
	Usage:     "Create chunks",
	ArgsUsage: "<outputPath>",
	Action:    CreateChunks,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "json",
			Usage:    "The meta file",
			Required: true,
		},
		&cli.StringFlag{
			Name:     "parent",
			Usage:    "The meta file",
			Required: true,
		},
	},
}

// Refer to the boostx code at github.com/filecoin-project/boost/cmd/boostx/utils_cmd.go for functional validation.
func CreateChunks(cctx *cli.Context) error {
	if cctx.Args().Len() != 1 {
		return xerrors.Errorf("usage: create-chunks <outputPath>")
	}

	outPath := cctx.Args().First()

	msrv := metaservice.New()

	if err := msrv.LoadMeta(cctx.String("json")); err != nil {
		return err
	}
	metas, err := msrv.GetAllChunkMetas()
	if err != nil {
		return err
	}
	return msrv.GenerateChunksFromMeta(outPath, cctx.String("parent"), metas)
}
