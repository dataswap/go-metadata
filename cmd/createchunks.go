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
			Name:     "mapping-file",
			Usage:    "The meta mapping file to write to",
			Required: true,
		},
		&cli.StringFlag{
			Name:     "source-parent-path",
			Usage:    "The source data parent path",
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

	if err := msrv.LoadMetaMappings(cctx.String("mapping-file")); err != nil {
		return err
	}
	metas, err := msrv.GetAllChunkMappings()
	if err != nil {
		return err
	}
	return msrv.GenerateChunksFromMappings(outPath, cctx.String("source-parent-path"), metas)
}
