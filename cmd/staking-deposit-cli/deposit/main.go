package main

import (
	"github.com/cyyber/qrysm/v4/cmd/staking-deposit-cli/deposit/existingseed"
	"github.com/cyyber/qrysm/v4/cmd/staking-deposit-cli/deposit/newseed"
	"os"

	"github.com/urfave/cli/v2"
)

var depositCommands []*cli.Command

func main() {
	app := &cli.App{
		Commands: depositCommands,
	}
	err := app.Run(os.Args)
	if err != nil {
		panic(err)
	}
}

func init() {
	depositCommands = append(depositCommands, existingseed.Commands...)
	depositCommands = append(depositCommands, newseed.Commands...)
}
