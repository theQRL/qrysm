package main

import (
	"os"

	"github.com/cyyber/qrysm/v4/cmd/staking-deposit-cli/deposit/existingseed"
	"github.com/cyyber/qrysm/v4/cmd/staking-deposit-cli/deposit/generatedilithiumtoexecutionchange"
	"github.com/cyyber/qrysm/v4/cmd/staking-deposit-cli/deposit/newseed"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var depositCommands []*cli.Command

func main() {
	app := &cli.App{
		Commands: depositCommands,
	}
	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

func init() {
	depositCommands = append(depositCommands, existingseed.Commands...)
	depositCommands = append(depositCommands, newseed.Commands...)
	depositCommands = append(depositCommands, generatedilithiumtoexecutionchange.Commands...)
}
