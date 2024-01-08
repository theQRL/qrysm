package submit

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
	dilithiumlib "github.com/theQRL/go-qrllib/dilithium"
	"github.com/theQRL/go-zond/accounts/abi/bind"
	"github.com/theQRL/go-zond/common"
	"github.com/theQRL/go-zond/core/types"
	"github.com/theQRL/go-zond/rpc"
	"github.com/theQRL/go-zond/zondclient"
	"github.com/theQRL/qrysm/v4/cmd/staking-deposit-cli/deposit/flags"
	"github.com/theQRL/qrysm/v4/cmd/staking-deposit-cli/misc"
	"github.com/theQRL/qrysm/v4/cmd/staking-deposit-cli/stakingdeposit"
	"github.com/theQRL/qrysm/v4/contracts/deposit"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	"github.com/urfave/cli/v2"
)

// TODO(rgeraldes24): value validation?
// TODO(rgeraldes24): operation timeout
// TODO(rgeraldes24): gas fees
// TODO(rgeraldes24): check if the deposit already exists?
// TODO(rgeraldes24): wait for tx confirmation option?
// TODO(rgeraldes24): delay between transactions?

const depositDataFilePrefix = "deposit_data-"

func submitDeposits(cliCtx *cli.Context) error {

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
	contractAddr := cliCtx.String(flags.DepositContractAddressFlag.Name)
	contract, err := deposit.NewDepositContract(common.HexToAddress(contractAddr), zondCli)
	if err != nil {
		return fmt.Errorf("failed to create a new instance of the deposit contract. reason: %v", err)
	}

	validatorKeysDir := cliCtx.String(flags.HTTPWeb3ProviderFlag.Name)
	depositDataList, err := importDepositDataJSON(validatorKeysDir)
	if err != nil {
		return fmt.Errorf("failed to read deposit data. reason: %v", err)
	}

	signingSeedFile := cliCtx.String(flags.ZondSeedFileFlag.Name)
	binSigningSeed, err := os.ReadFile(signingSeedFile)
	if err != nil {
		return fmt.Errorf("failed to read seed file. reason: %v", err)
	}
	depositKey, err := dilithiumlib.NewDilithiumFromSeed(bytesutil.ToBytes48(binSigningSeed))
	if err != nil {
		return fmt.Errorf("failed to generate the deposit key from the signing seed. reason: %v", err)
	}

	for _, depositData := range depositDataList {
		tx, err := sendDepositTx(contract, depositKey, depositData, chainID)
		if err != nil {
			return fmt.Errorf("failed to submit deposit transaction. reason: %v", err)
		}

		log.WithFields(logrus.Fields{
			"Transaction Hash": fmt.Sprintf("%#x", tx.Hash()),
		}).Infof(
			"Deposit sent to contract address %v for validator with a public key %#x",
			contractAddr,
			depositData.PubKey,
		)
	}

	return nil
}

func sendDepositTx(
	contract *deposit.DepositContract,
	key *dilithiumlib.Dilithium,
	data *stakingdeposit.DepositData,
	chainID *big.Int,
) (*types.Transaction, error) {
	txOpts, err := bind.NewKeyedTransactorWithChainID(key, chainID)
	if err != nil {
		return nil, err
	}

	txOpts.Value = new(big.Int).Mul(big.NewInt(int64(data.Amount)), big.NewInt(1e9)) // value in wei
	txOpts.GasLimit = 500000
	txOpts.GasFeeCap = new(big.Int).SetUint64(50000)
	txOpts.GasTipCap = new(big.Int).SetUint64(50000)

	tx, err := contract.Deposit(
		txOpts,
		misc.DecodeHex(data.PubKey),
		misc.DecodeHex(data.WithdrawalCredentials),
		misc.DecodeHex(data.Signature),
		bytesutil.ToBytes32(misc.DecodeHex(data.DepositDataRoot)),
	)
	if err != nil {
		return nil, err
	}

	return tx, nil
}

func importDepositDataJSON(folder string) ([]*stakingdeposit.DepositData, error) {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return nil, err
	}

	var file string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(depositDataFilePrefix, entry.Name()) {
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
