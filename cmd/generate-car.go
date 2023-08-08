package main

import (
	"io"
	"os"

	llog "log"

	"github.com/urfave/cli/v2"

	logging "github.com/ipfs/go-log/v2"
)

var log = logging.Logger("generate-car")

func init() {
	llog.SetOutput(io.Discard)
}

func main() { os.Exit(main1()) }

func main1() int {
	app := &cli.App{
		Name:     "noah",
		Usage:    "Utility for working with car files",
		Before:   before,
		Commands: []*cli.Command{
			//generateCarCmd,
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Error(err)
		return 1
	}
	return 0
}

func before(cctx *cli.Context) error {
	_ = logging.SetLogLevel("noah", "INFO")
	return nil
}
