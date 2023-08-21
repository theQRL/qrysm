package existingseed

import (
	"github.com/cyyber/qrysm/v4/cmd/staking-deposit-cli/stakingdeposit"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var (
	existingSeedFlags = struct {
		Seed                string
		ValidatorStartIndex uint64
		NumValidators       uint64
		Folder              string
		ChainName           string
		KeystorePassword    string
		ExecutionAddress    string
	}{}
	log = logrus.WithField("prefix", "deposit")
)

var Commands = []*cli.Command{
	{
		Name:    "existing-seed",
		Aliases: []string{"exst-seed"},
		Usage:   "",
		Action: func(cliCtx *cli.Context) error {
			if err := cliActionExistingSeed(cliCtx); err != nil {
				log.WithError(err).Fatal("Could not generate using an existing seed")
			}
			return nil
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "seed",
				Usage:       "",
				Destination: &existingSeedFlags.Seed,
				Required:    true,
			},
			&cli.Uint64Flag{
				Name:        "validator-start-index",
				Usage:       "",
				Destination: &existingSeedFlags.ValidatorStartIndex,
				Required:    true,
			},
			&cli.Uint64Flag{
				Name:        "num-validators",
				Usage:       "",
				Destination: &existingSeedFlags.NumValidators,
				Required:    true,
			},
			&cli.StringFlag{
				Name:        "folder",
				Usage:       "",
				Destination: &existingSeedFlags.Folder,
				Required:    true,
			},
			&cli.StringFlag{
				Name:        "chain-name",
				Usage:       "",
				Destination: &existingSeedFlags.ChainName,
				Value:       "betanet",
			},
			&cli.StringFlag{
				Name:        "keystore-password",
				Usage:       "",
				Destination: &existingSeedFlags.KeystorePassword,
				Required:    true,
			},
			&cli.StringFlag{
				Name:        "execution-address",
				Usage:       "",
				Destination: &existingSeedFlags.ExecutionAddress,
				Value:       "betanet",
			},
		},
		Subcommands: []*cli.Command{
			nil,
		},
	},
}

func cliActionExistingSeed(cliCtx *cli.Context) error {
	// TODO: (cyyber) Replace seed by mnemonic
	stakingdeposit.GenerateKeys(existingSeedFlags.ValidatorStartIndex,
		existingSeedFlags.NumValidators, existingSeedFlags.Seed, existingSeedFlags.Folder,
		existingSeedFlags.ChainName, existingSeedFlags.KeystorePassword, existingSeedFlags.ExecutionAddress)

	return nil
}
