package newseed

import (
	"fmt"
	"strings"
	"syscall"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/theQRL/go-qrl/common"
	goqrllib_misc "github.com/theQRL/go-qrllib/wallet/misc"
	"github.com/theQRL/qrysm/cmd/staking-deposit-cli/misc"
	"github.com/theQRL/qrysm/cmd/staking-deposit-cli/stakingdeposit"
	"github.com/theQRL/qrysm/cmd/staking-deposit-cli/stakingdeposit/keyhandling/keyderivation"
	"github.com/theQRL/qrysm/io/file"
	"github.com/urfave/cli/v2"
	"golang.org/x/term"
)

var (
	newSeedFlags = struct {
		ValidatorStartIndex uint64
		NumValidators       uint64
		Folder              string
		ChainName           string
		ExecutionAddress    string
		Mnemonic            string
		LightKDF            bool
	}{}
	log = logrus.WithField("prefix", "deposit")

	// KeystorePasswordFile is the path to a file containing the keystore password.
	KeystorePasswordFile = &cli.StringFlag{
		Name:  "keystore-password-file",
		Usage: "The keystore password.",
	}
)
var Commands = []*cli.Command{
	{
		Name:    "new-seed",
		Aliases: []string{"ns"},
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
				Value:       0,
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
				Value:       "validator_keys",
			},
			&cli.StringFlag{
				Name:        "chain-name",
				Usage:       "",
				Destination: &newSeedFlags.ChainName,
				Value:       "testnet",
			},
			&cli.StringFlag{
				Name:        "execution-address",
				Usage:       "",
				Destination: &newSeedFlags.ExecutionAddress,
				Value:       "",
				Required:    true,
			},
			&cli.StringFlag{
				Name:        "mnemonic",
				Usage:       "",
				Destination: &newSeedFlags.Mnemonic,
				Value:       "",
			},
			&cli.BoolFlag{
				Name:        "lightkdf",
				Usage:       "Reduce key-derivation RAM & CPU usage at some expense of KDF strength",
				Destination: &newSeedFlags.LightKDF,
			},
			KeystorePasswordFile,
		},
	},
}

func cliActionNewSeed(cliCtx *cli.Context) error {
	var keystorePassword string
	if cliCtx.IsSet(KeystorePasswordFile.Name) {
		passwordFilePathInput := cliCtx.String(KeystorePasswordFile.Name)
		data, err := file.ReadFileAsBytes(passwordFilePathInput)
		if err != nil {
			return errors.Wrap(err, "could not read file as bytes")
		}
		keystorePassword = strings.TrimRight(string(data), "\r\n")
	} else {
		fmt.Println("Create a password that secures your validator keystore(s). " +
			"You will need to re-enter this to decrypt them when you setup your QRL validators.")
		password, err := term.ReadPassword(int(syscall.Stdin))
		if err != nil {
			return err
		}

		fmt.Println("Re-enter password ")
		reEnterKeystorePassword, err := term.ReadPassword(int(syscall.Stdin))
		if err != nil {
			return err
		}

		if string(password) != string(reEnterKeystorePassword) {
			return fmt.Errorf("password mismatch")
		}
		keystorePassword = string(password)
	}

	mnemonic := newSeedFlags.Mnemonic
	if len(mnemonic) == 0 {
		mnemonic = keyderivation.GetRandomMnemonic()
	}

	seed, err := goqrllib_misc.MnemonicToBin(mnemonic)
	if err != nil {
		return err
	}

	executionAddr, err := common.NewAddressFromString(newSeedFlags.ExecutionAddress)
	if err != nil {
		return err
	}

	stakingdeposit.GenerateKeys(newSeedFlags.ValidatorStartIndex,
		newSeedFlags.NumValidators, misc.EncodeHex(seed[:]), newSeedFlags.Folder,
		newSeedFlags.ChainName, keystorePassword, executionAddr,
		newSeedFlags.LightKDF)

	return nil
}
