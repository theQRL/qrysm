package submit

import (
	"github.com/sirupsen/logrus"
	"github.com/theQRL/qrysm/v4/cmd/staking-deposit-cli/deposit/flags"
	"github.com/urfave/cli/v2"
)

var log = logrus.WithField("prefix", "submit")

var Command = &cli.Command{
	Name: "submit",
	Description: "Submits deposits to the zond deposit contract for a set of validators by connecting " +
		"to a zond endpoint to submit the transactions. Requires signing the transactions with a zond private key",
	Usage: "",
	Action: func(cliCtx *cli.Context) error {
		return submitDeposits(cliCtx)
	},
	Flags: []cli.Flag{
		flags.ValidatorKeysDirFlag,
		flags.ZondSeedFileFlag,
		flags.DepositContractAddressFlag,
		flags.HTTPWeb3ProviderFlag,
	},
}
