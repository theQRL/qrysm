package existingseed

import (
	"encoding/hex"
	"fmt"
	"syscall"

	"github.com/sirupsen/logrus"
	"github.com/theQRL/go-qrl/common"
	"github.com/theQRL/go-qrllib/wallet/common/descriptor"
	"github.com/theQRL/go-qrllib/wallet/common/wallettype"
	"github.com/theQRL/qrysm/cmd/staking-deposit-cli/stakingdeposit"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/urfave/cli/v2"
	"golang.org/x/term"
)

var (
	existingSeedFlags = struct {
		ExtendedSeed        string
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
				Name:        "extended-seed",
				Usage:       "",
				Destination: &existingSeedFlags.ExtendedSeed,
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
				Required:    true,
			},
		},
	},
}

func cliActionExistingSeed(cliCtx *cli.Context) error {
	// TODO: (cyyber) Replace seed by mnemonic

	fmt.Println("Create a password that secures your validator keystore(s). " +
		"You will need to re-enter this to decrypt them when you setup your QRL validators.")
	keystorePassword, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return err
	}

	fmt.Println("Re-enter password ")
	reEnterKeystorePassword, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return err
	}

	if string(keystorePassword) != string(reEnterKeystorePassword) {
		return fmt.Errorf("password mismatch")
	}

	executionAddr, err := common.NewAddressFromString(existingSeedFlags.ExecutionAddress)
	if err != nil {
		return err
	}

	extendedSeed := existingSeedFlags.ExtendedSeed
	if extendedSeed[:2] == "0x" {
		extendedSeed = extendedSeed[2:]
	}

	binExtendedSeed, err := hex.DecodeString(extendedSeed)
	if err != nil {
		return err
	}

	if len(binExtendedSeed) != fieldparams.ExtendedSeedLength {
		return fmt.Errorf("invalid extended seed length | expected: %d, actual: %d",
			fieldparams.ExtendedSeedLength, len(binExtendedSeed))
	}

	d, err := descriptor.FromBytes(binExtendedSeed[:descriptor.DescriptorSize])
	if err != nil {
		return err
	}

	// only ML-DSA-87 wallet type is supported for staking
	if wallettype.WalletType(d.Type()) != wallettype.ML_DSA_87 {
		panic("expected wallet type ML-DSA-87")
	}

	seed := hex.EncodeToString(binExtendedSeed[descriptor.DescriptorSize:])
	stakingdeposit.GenerateKeys(existingSeedFlags.ValidatorStartIndex,
		existingSeedFlags.NumValidators, seed, existingSeedFlags.Folder,
		existingSeedFlags.ChainName, string(keystorePassword), executionAddr, false)

	return nil
}
