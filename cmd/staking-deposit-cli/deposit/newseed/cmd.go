package newseed

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/cyyber/qrysm/v4/cmd/staking-deposit-cli/stakingdeposit"
	"github.com/sirupsen/logrus"
	"github.com/theQRL/go-qrllib/common"
	"github.com/urfave/cli/v2"
)

var (
	newSeedFlags = struct {
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
		Name:    "new-seed",
		Aliases: []string{"new-seed"},
		Usage:   "",
		Action: func(cliCtx *cli.Context) error {
			if err := cliActionNewSeed(cliCtx); err != nil {
				log.WithError(err).Fatal("Could not generate new seed")
			}
			return nil
		},
		Flags: []cli.Flag{
			&cli.Uint64Flag{
				Name:        "validator-start-index",
				Usage:       "",
				Destination: &newSeedFlags.ValidatorStartIndex,
				Required:    true,
			},
			&cli.Uint64Flag{
				Name:        "num-validators",
				Usage:       "",
				Destination: &newSeedFlags.NumValidators,
				Required:    true,
			},
			&cli.StringFlag{
				Name:        "folder",
				Usage:       "",
				Destination: &newSeedFlags.Folder,
				Required:    true,
			},
			&cli.StringFlag{
				Name:        "chain-name",
				Usage:       "",
				Destination: &newSeedFlags.ChainName,
				Value:       "betanet",
			},
			&cli.StringFlag{
				Name:        "keystore-password",
				Usage:       "",
				Destination: &newSeedFlags.KeystorePassword,
				Required:    true,
			},
			&cli.StringFlag{
				Name:        "execution-address",
				Usage:       "",
				Destination: &newSeedFlags.ExecutionAddress,
				Value:       "betanet",
			},
		},
	},
}

func cliActionNewSeed(cliCtx *cli.Context) error {
	// TODO: (cyyber) Replace seed by mnemonic
	var seed [common.SeedSize]uint8

	_, err := rand.Read(seed[:])
	if err != nil {
		return fmt.Errorf("failed to generate random seed for Dilithium address: %v", err)
	}

	stakingdeposit.GenerateKeys(newSeedFlags.ValidatorStartIndex,
		newSeedFlags.NumValidators, hex.EncodeToString(seed[:]), newSeedFlags.Folder,
		newSeedFlags.ChainName, newSeedFlags.KeystorePassword, newSeedFlags.ExecutionAddress)

	return nil
}
