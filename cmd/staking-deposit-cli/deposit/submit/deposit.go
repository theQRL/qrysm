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

	"github.com/sirupsen/logrus"
	"github.com/theQRL/go-qrl/accounts/abi/bind"
	"github.com/theQRL/go-qrl/common"
	"github.com/theQRL/go-qrl/crypto/pqcrypto/wallet"
	"github.com/theQRL/go-qrl/qrlclient"
	"github.com/theQRL/go-qrl/rpc"
	"github.com/theQRL/qrysm/cmd"
	"github.com/theQRL/qrysm/cmd/staking-deposit-cli/deposit/flags"
	"github.com/theQRL/qrysm/cmd/staking-deposit-cli/stakingdeposit"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/contracts/deposit"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/monitoring/progress"
	"github.com/urfave/cli/v2"
)

const depositDataFilePrefix = "deposit_data-"

func submitDeposits(cliCtx *cli.Context) error {
	validatorKeysDir := cliCtx.String(flags.ValidatorKeysDirFlag.Name)
	depositDataList, err := importDepositDataJSON(validatorKeysDir)
	if err != nil {
		return fmt.Errorf("failed to read deposit data. reason: %v", err)
	}

	contractAddrStr := cliCtx.String(flags.DepositContractAddressFlag.Name)
	if !cliCtx.Bool(flags.SkipDepositConfirmationFlag.Name) {
		qrlDepositTotal := uint64(len(depositDataList)) * params.BeaconConfig().MaxEffectiveBalance / params.BeaconConfig().ShorPerQuanta
		actionText := "This will submit the deposits stored in your deposit data directory. " +
			fmt.Sprintf("A total of %d QRL will be sent to contract address %s for %d validator accounts. ", qrlDepositTotal, contractAddrStr, len(depositDataList)) +
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
		return fmt.Errorf("failed to connect to the qrl provider. reason: %v", err)
	}
	qrlCli := qrlclient.NewClient(rpcClient)
	chainID, err := qrlCli.ChainID(cliCtx.Context)
	if err != nil {
		return fmt.Errorf("failed to retrieve the chain ID. reason: %v", err)
	}
	contractAddr, err := common.NewAddressFromString(contractAddrStr)
	if err != nil {
		return err
	}
	contract, err := deposit.NewDepositContract(contractAddr, qrlCli)
	if err != nil {
		return fmt.Errorf("failed to create a new instance of the deposit contract. reason: %v", err)
	}

	signingSeedFile := cliCtx.String(flags.QRLSeedFileFlag.Name)
	wallet, err := wallet.RestoreFromFile(signingSeedFile)
	if err != nil {
		return fmt.Errorf("failed to restore wallet from file: %v", err)
	}

	gasTip, err := qrlCli.SuggestGasTipCap(cliCtx.Context)
	if err != nil {
		return fmt.Errorf("failed to get gas tip suggestion. reason: %v", err)
	}

	txOpts, err := bind.NewKeyedTransactorWithChainID(wallet, chainID)
	if err != nil {
		return err
	}
	txOpts.GasLimit = 500000
	txOpts.GasFeeCap = nil
	txOpts.GasTipCap = gasTip

	depositDelaySeconds := cliCtx.Int(flags.DepositDelaySecondsFlag.Name)
	depositDelay := time.Duration(depositDelaySeconds) * time.Second
	bar := progress.InitializeProgressBar(len(depositDataList), "Sending deposit transactions...")
	for i, depositData := range depositDataList {
		txOpts.Value = new(big.Int).Mul(new(big.Int).SetUint64(depositData.Amount), big.NewInt(1e9)) // value in planck

		if err := sendDepositTx(contract, depositData, txOpts); err != nil {
			log.WithError(err).Errorf("Unable to send transaction to contract: deposit data index: %d", i)
			continue
		}

		log.Infof("Waiting for a short delay of %v seconds...", depositDelaySeconds)
		if err := bar.Add(1); err != nil {
			log.WithError(err).Error("Could not increase progress bar percentage")
		}
		time.Sleep(depositDelay)
	}

	log.Infof("Successfully sent all validator deposits!")

	return nil
}

func sendDepositTx(
	contract *deposit.DepositContract,
	data *stakingdeposit.DepositData,
	txOpts *bind.TransactOpts,
) error {
	pubKeyBytes, err := hex.DecodeString(data.PubKey[2:])
	if err != nil {
		return err
	}
	credsBytes, err := hex.DecodeString(data.WithdrawalCredentials[2:])
	if err != nil {
		return err
	}
	sigBytes, err := hex.DecodeString(data.Signature[2:])
	if err != nil {
		return err
	}
	depDataRootBytes, err := hex.DecodeString(data.DepositDataRoot[2:])
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
