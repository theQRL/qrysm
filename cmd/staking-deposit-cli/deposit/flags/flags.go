package flags

import (
	"github.com/urfave/cli/v2"
)

// ValidatorKeysDefaultDirName for validator_keys.
const ValidatorKeysDefaultDirName = "validator_keys"

var (
	// ValidatorKeysDir defines the path to the validator keys directory.
	ValidatorKeysDirFlag = &cli.StringFlag{
		Name:  "validator-keys-dir",
		Usage: "Path to a wallet directory on-disk for validator keys",
		// Value: filepath.Join(DefaultValidatorKeysDir(), ValidatorKeysDefaultDirName),
		Value: ValidatorKeysDefaultDirName,
	}
	// ZondSeedFileFlag for transaction signing.
	ZondSeedFileFlag = &cli.StringFlag{
		Name:     "zond-seed-file",
		Usage:    "File containing a seed for sending deposit transactions from zond",
		Value:    "",
		Required: true,
	}
	// HTTPWeb3ProviderFlag provides an HTTP access endpoint to a ZOND RPC.
	HTTPWeb3ProviderFlag = &cli.StringFlag{
		Name:  "http-web3provider",
		Usage: "A zond web3 provider string http endpoint",
		Value: "http://localhost:8545",
	}
	// DepositContractAddressFlag for the validator deposit contract on zond.
	DepositContractAddressFlag = &cli.StringFlag{
		Name:  "deposit-contract",
		Usage: "Address of the deposit contract",
		Value: "Z4242424242424242424242424242424242424242", // TODO (cyyber): Replace this with params
	}
	// SkipDepositConfirmationFlag skips the y/n confirmation prompt for sending a deposit to the deposit contract.
	SkipDepositConfirmationFlag = &cli.BoolFlag{
		Name:  "skip-deposit-confirmation",
		Usage: "Skips the y/n confirmation prompt for sending a deposit to the deposit contract",
		Value: false,
	}
	// DepositDelaySecondsFlag to delay sending deposit transactions by a fixed interval.
	DepositDelaySecondsFlag = &cli.Int64Flag{
		Name:  "deposit-delay-seconds",
		Usage: "The time delay between sending the deposits to the contract (in seconds)",
		Value: 5,
	}
)
