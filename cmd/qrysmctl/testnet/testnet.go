package testnet

import "github.com/urfave/cli/v2"

var Commands = []*cli.Command{
	{
		Name:  "testnet",
		Usage: "commands for dealing with Zond beacon chain testnets",
		Subcommands: []*cli.Command{
			generateGenesisStateCmd,
		},
	},
}
