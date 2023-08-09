package main

import (
	"os"

	"github.com/urfave/cli/v2"

	logging "github.com/ipfs/go-log/v2"
)

var log = logging.Logger("generate-car")

func main() {
	app := &cli.App{
		Name:   "generate-car",
		Usage:  "Utility for working with car files",
		Before: before,
		Commands: []*cli.Command{
			//generateCarCmd,
			listCmd,
			createCmd,
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Error(err)
		return
	}
	return
}

func before(cctx *cli.Context) error {
	_ = logging.SetLogLevel("generate-car", "INFO")
	return nil
}
