package main

import (
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/theQRL/qrysm/cmd/qrysmctl/checkpointsync"
	"github.com/theQRL/qrysm/cmd/qrysmctl/db"
	"github.com/theQRL/qrysm/cmd/qrysmctl/p2p"
	"github.com/theQRL/qrysm/cmd/qrysmctl/testnet"
	"github.com/theQRL/qrysm/cmd/qrysmctl/validator"
	"github.com/theQRL/qrysm/cmd/qrysmctl/weaksubjectivity"
	"github.com/urfave/cli/v2"
)

var qrysmctlCommands []*cli.Command

func main() {
	app := &cli.App{
		Commands: qrysmctlCommands,
	}
	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

func init() {
	qrysmctlCommands = append(qrysmctlCommands, checkpointsync.Commands...)
	qrysmctlCommands = append(qrysmctlCommands, db.Commands...)
	qrysmctlCommands = append(qrysmctlCommands, p2p.Commands...)
	qrysmctlCommands = append(qrysmctlCommands, testnet.Commands...)
	qrysmctlCommands = append(qrysmctlCommands, weaksubjectivity.Commands...)
	qrysmctlCommands = append(qrysmctlCommands, validator.Commands...)
}
