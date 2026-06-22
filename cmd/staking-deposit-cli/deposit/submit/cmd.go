package submit

import (
	"github.com/sirupsen/logrus"
	"github.com/theQRL/qrysm/cmd/staking-deposit-cli/deposit/flags"
	"github.com/urfave/cli/v2"
)

var log = logrus.WithField("prefix", "deposit")

var Command = &cli.Command{
	Name: "submit",
	Description: "Submits deposits to the qrl deposit contract for a set of validators by connecting " +
		"to a qrl execution node endpoint to submit the transactions. Requires signing the transactions with a qrl private key",
	Usage: "",
	Action: func(cliCtx *cli.Context) error {
		return submitDeposits(cliCtx)
	},
	Flags: []cli.Flag{
		flags.ValidatorKeysDirFlag,
		flags.QRLSeedFileFlag,
		flags.DepositContractAddressFlag,
		flags.HTTPWeb3ProviderFlag,
		flags.DepositDelaySecondsFlag,
		flags.SkipDepositConfirmationFlag,
	},
}
