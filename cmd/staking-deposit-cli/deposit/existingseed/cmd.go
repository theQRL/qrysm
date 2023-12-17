package existingseed

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"github.com/theQRL/qrysm/v4/cmd/staking-deposit-cli/stakingdeposit"
	"github.com/urfave/cli/v2"
	"golang.org/x/term"
	"syscall"
)

var (
	existingSeedFlags = struct {
		Seed                string
		ValidatorStartIndex uint64
		NumValidators       uint64
		Folder              string
		ChainName           string
		ExecutionAddress    string
	}{}
	log = logrus.WithField("prefix", "existing-seed")
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
				Value:       0,
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
				Value:       "validator_keys",
			},
			&cli.StringFlag{
				Name:        "chain-name",
				Usage:       "",
				Destination: &existingSeedFlags.ChainName,
				Value:       "betanet",
			},
			&cli.StringFlag{
				Name:        "execution-address",
				Usage:       "",
				Destination: &existingSeedFlags.ExecutionAddress,
				Value:       "",
			},
		},
	},
}

func cliActionExistingSeed(cliCtx *cli.Context) error {
	// TODO: (cyyber) Replace seed by mnemonic

	fmt.Println("Create a password that secures your validator keystore(s). " +
		"You will need to re-enter this to decrypt them when you setup your Zond validators.")
	keystorePassword, err := term.ReadPassword(syscall.Stdin)
	if err != nil {
		return err
	}

	fmt.Println("Re-enter password ")
	reEnterKeystorePassword, err := term.ReadPassword(syscall.Stdin)
	if err != nil {
		return err
	}

	if string(keystorePassword) != string(reEnterKeystorePassword) {
		return fmt.Errorf("password mismatch")
	}

	stakingdeposit.GenerateKeys(existingSeedFlags.ValidatorStartIndex,
		existingSeedFlags.NumValidators, existingSeedFlags.Seed, existingSeedFlags.Folder,
		existingSeedFlags.ChainName, string(keystorePassword), existingSeedFlags.ExecutionAddress)

	return nil
}
