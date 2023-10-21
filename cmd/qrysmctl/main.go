package main

import (
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/theQRL/qrysm/v4/cmd/qrysmctl/checkpointsync"
	"github.com/theQRL/qrysm/v4/cmd/qrysmctl/db"
	"github.com/theQRL/qrysm/v4/cmd/qrysmctl/deprecated"
	"github.com/theQRL/qrysm/v4/cmd/qrysmctl/p2p"
	"github.com/theQRL/qrysm/v4/cmd/qrysmctl/testnet"
	"github.com/theQRL/qrysm/v4/cmd/qrysmctl/validator"
	"github.com/theQRL/qrysm/v4/cmd/qrysmctl/weaksubjectivity"
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
	// contains the old checkpoint sync subcommands. these commands should display help/warn messages
	// pointing to their new locations
	qrysmctlCommands = append(qrysmctlCommands, deprecated.Commands...)

	qrysmctlCommands = append(qrysmctlCommands, checkpointsync.Commands...)
	qrysmctlCommands = append(qrysmctlCommands, db.Commands...)
	qrysmctlCommands = append(qrysmctlCommands, p2p.Commands...)
	qrysmctlCommands = append(qrysmctlCommands, testnet.Commands...)
	qrysmctlCommands = append(qrysmctlCommands, weaksubjectivity.Commands...)
	qrysmctlCommands = append(qrysmctlCommands, validator.Commands...)
}
