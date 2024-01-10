package submit

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/k0kubun/go-ansi"
	"github.com/schollz/progressbar/v3"
	"github.com/sirupsen/logrus"
	dilithiumlib "github.com/theQRL/go-qrllib/dilithium"
	"github.com/theQRL/go-zond/accounts/abi/bind"
	"github.com/theQRL/go-zond/common"
	"github.com/theQRL/go-zond/rpc"
	"github.com/theQRL/go-zond/zondclient"
	"github.com/theQRL/qrysm/v4/cmd"
	"github.com/theQRL/qrysm/v4/cmd/staking-deposit-cli/deposit/flags"
	"github.com/theQRL/qrysm/v4/cmd/staking-deposit-cli/stakingdeposit"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/contracts/deposit"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	"github.com/urfave/cli/v2"
)

const depositDataFilePrefix = "deposit_data-"

func submitDeposits(cliCtx *cli.Context) error {
	validatorKeysDir := cliCtx.String(flags.ValidatorKeysDirFlag.Name)
	depositDataList, err := importDepositDataJSON(validatorKeysDir)
	if err != nil {
		return fmt.Errorf("failed to read deposit data. reason: %v", err)
	}

	contractAddr := cliCtx.String(flags.DepositContractAddressFlag.Name)
	if !cliCtx.Bool(flags.SkipDepositConfirmationFlag.Name) {
		qrlDepositTotal := uint64(len(depositDataList)) * params.BeaconConfig().MaxEffectiveBalance / params.BeaconConfig().GweiPerEth
		actionText := "This will submit the deposits stored in your deposit data directory. " +
			fmt.Sprintf("A total of %d QRL will be sent to contract address %s for %d validator accounts. ", qrlDepositTotal, contractAddr, len(depositDataList)) +
			"Do you want to proceed? (Y/N)"
		deniedText := "Deposits will not be submitted. No changes have been made."
		submitConfirmed, err := cmd.ConfirmAction(actionText, deniedText)
		if err != nil {
			return err
		}
		if !submitConfirmed {
			return nil
		}
	}

	web3Provider := cliCtx.String(flags.HTTPWeb3ProviderFlag.Name)
	rpcClient, err := rpc.Dial(web3Provider)
	if err != nil {
		return fmt.Errorf("failed to connect to the zond provider. reason: %v", err)
	}
	zondCli := zondclient.NewClient(rpcClient)
	chainID, err := zondCli.ChainID(cliCtx.Context)
	if err != nil {
		return fmt.Errorf("failed to retrieve the chain ID. reason: %v", err)
	}
	contract, err := deposit.NewDepositContract(common.HexToAddress(contractAddr), zondCli)
	if err != nil {
		return fmt.Errorf("failed to create a new instance of the deposit contract. reason: %v", err)
	}

	signingSeedFile := cliCtx.String(flags.ZondSeedFileFlag.Name)
	signingSeedHex, err := os.ReadFile(signingSeedFile)
	if err != nil {
		return fmt.Errorf("failed to read seed file. reason: %v", err)
	}
	signingSeed := make([]byte, hex.DecodedLen(len(signingSeedHex)))
	_, err = hex.Decode(signingSeed, signingSeedHex)
	if err != nil {
		return fmt.Errorf("failed to read seed. reason: %v", err)
	}

	depositKey, err := dilithiumlib.NewDilithiumFromSeed(bytesutil.ToBytes48(signingSeed))
	if err != nil {
		return fmt.Errorf("failed to generate the deposit key from the signing seed. reason: %v", err)
	}

	gasTip, err := zondCli.SuggestGasTipCap(cliCtx.Context)
	if err != nil {
		return fmt.Errorf("failed to get gas tip suggestion. reason: %v", err)
	}

	txOpts, err := bind.NewKeyedTransactorWithChainID(depositKey, chainID)
	if err != nil {
		return err
	}
	txOpts.GasLimit = 500000
	txOpts.GasFeeCap = nil
	txOpts.GasTipCap = gasTip
	txOpts.Value = new(big.Int).Mul(big.NewInt(int64(params.BeaconConfig().MaxEffectiveBalance)), big.NewInt(1e9)) // value in wei

	depositDelaySeconds := cliCtx.Int(flags.DepositDelaySecondsFlag.Name)
	depositDelay := time.Duration(depositDelaySeconds) * time.Second
	bar := initializeProgressBar(len(depositDataList), "Sending deposit transactions...")
	for i, depositData := range depositDataList {
		if err := sendDepositTx(contract, depositData, chainID, txOpts); err != nil {
			log.Errorf("Unable to send transaction to contract: %v | deposit data index: %d", err, i)
			continue
		}

		log.Infof("Waiting for a short delay of %v seconds...", depositDelaySeconds)
		if err := bar.Add(1); err != nil {
			log.Errorf("Could not increase progress bar percentage: %v", err)
		}
		time.Sleep(depositDelay)
	}

	log.Infof("Successfully sent all validator deposits!")

	return nil
}

func sendDepositTx(
	contract *deposit.DepositContract,
	data *stakingdeposit.DepositData,
	chainID *big.Int,
	txOpts *bind.TransactOpts,
) error {
	pubKeyBytes, err := hex.DecodeString(data.PubKey)
	if err != nil {
		return err
	}
	credsBytes, err := hex.DecodeString(data.WithdrawalCredentials)
	if err != nil {
		return err
	}
	sigBytes, err := hex.DecodeString(data.Signature)
	if err != nil {
		return err
	}
	depDataRootBytes, err := hex.DecodeString(data.Signature)
	if err != nil {
		return err
	}

	tx, err := contract.Deposit(
		txOpts,
		pubKeyBytes,
		credsBytes,
		sigBytes,
		bytesutil.ToBytes32(depDataRootBytes),
	)
	if err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		"Transaction Hash": fmt.Sprintf("%#x", tx.Hash()),
	}).Info("Deposit sent for validator")

	return nil
}

func importDepositDataJSON(folder string) ([]*stakingdeposit.DepositData, error) {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return nil, err
	}

	var file string
	for _, entry := range entries {
		fmt.Println(entry.Name())
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), depositDataFilePrefix) {
			file = entry.Name()
			break
		}
	}

	if file == "" {
		return nil, fmt.Errorf("deposit data file not found. dir: %s", folder)
	}

	fileFolder := filepath.Join(folder, file)
	data, err := os.ReadFile(fileFolder)
	if err != nil {
		return nil, err
	}

	var depositDataList []*stakingdeposit.DepositData
	if err := json.Unmarshal(data, &depositDataList); err != nil {
		return nil, fmt.Errorf("failed to read deposit data list. reason: %v", err)
	}

	return depositDataList, nil
}

func initializeProgressBar(numItems int, msg string) *progressbar.ProgressBar {
	return progressbar.NewOptions(
		numItems,
		progressbar.OptionFullWidth(),
		progressbar.OptionSetWriter(ansi.NewAnsiStdout()),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[green]=[reset]",
			SaucerHead:    "[green]>[reset]",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}),
		progressbar.OptionOnCompletion(func() { fmt.Println() }),
		progressbar.OptionSetDescription(msg),
	)
}
