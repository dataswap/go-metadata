package main

import (
	"os"

	"github.com/urfave/cli/v2"

	logging "github.com/ipfs/go-log/v2"
)

var log = logging.Logger("meta")

func main() {
	app := &cli.App{
		Name:   "meta",
		Usage:  "Utility for working with car files",
		Before: before,
		Commands: []*cli.Command{
			createCmd,
			listCmd,
			commpCmd,
			proofCmd,
			verifyCmd,
			tproofCmd,
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
	_ = logging.SetLogLevel("meta", "INFO")
	return nil
}
