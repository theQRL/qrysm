package main

import (
	"fmt"
	"math/big"
	"os"
	"time"

	"github.com/theQRL/go-qrl/params"
	"github.com/theQRL/qrysm/pkg/tx-fuzz/flags"
	"github.com/theQRL/qrysm/pkg/tx-fuzz/spammer"
	"github.com/urfave/cli/v2"
)

var airdropCommand = &cli.Command{
	Name:   "airdrop",
	Usage:  "Airdrops to a list of accounts",
	Action: runAirdrop,
	Flags: []cli.Flag{
		flags.SeedFlag,
		flags.RpcFlag,
	},
}

var spamCommand = &cli.Command{
	Name:   "spam",
	Usage:  "Send spam transactions",
	Action: runBasicSpam,
	Flags:  flags.SpamFlags,
}

var createCommand = &cli.Command{
	Name:   "create",
	Usage:  "Create ephemeral accounts",
	Action: runCreate,
	Flags: []cli.Flag{
		flags.CountFlag,
		flags.RpcFlag,
	},
}

var unstuckCommand = &cli.Command{
	Name:   "unstuck",
	Usage:  "Tries to unstuck an account",
	Action: runUnstuck,
	Flags:  flags.SpamFlags,
}

func initApp() *cli.App {
	app := cli.NewApp()
	app.Name = "tx-fuzz"
	app.Usage = "Fuzzer for sending spam transactions"
	app.Commands = []*cli.Command{
		airdropCommand,
		spamCommand,
		createCommand,
		unstuckCommand,
	}
	return app
}

var app = initApp()

func main() {
	// qrl.sendTransaction({from:personal.listAccounts[0], to:"Q00000000000000000000000000000000f1c4b3a519fb7cbb0c95143d22234411932151b9cc98b510d34bebb6f99f37abc1051bacd1853dc25bf92ead382cbaf7", value: "100000000000000"})
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runAirdrop(c *cli.Context) error {
	config, err := spammer.NewConfigFromContext(c)
	if err != nil {
		return err
	}
	txPerAccount := config.N
	airdropValue := new(big.Int).Mul(big.NewInt(int64(txPerAccount*100000)), big.NewInt(params.Shor))
	spammer.Airdrop(config, airdropValue)
	return nil
}

func spam(config *spammer.Config, spamFn spammer.Spam, airdropValue *big.Int) error {
	// Make sure the accounts are unstuck before sending any transactions
	spammer.Unstuck(config)
	for {
		if err := spammer.Airdrop(config, airdropValue); err != nil {
			return err
		}
		spammer.SpamTransactions(config, spamFn)
		time.Sleep(12 * time.Second)
	}
}

func runBasicSpam(c *cli.Context) error {
	config, err := spammer.NewConfigFromContext(c)
	if err != nil {
		return err
	}
	airdropValue := new(big.Int).Mul(big.NewInt(int64((1+config.N)*1000000)), big.NewInt(params.Shor))
	return spam(config, spammer.SendBasicTransactions, airdropValue)
}

func runCreate(c *cli.Context) error {
	spammer.CreateAddresses(100)
	return nil
}

func runUnstuck(c *cli.Context) error {
	config, err := spammer.NewConfigFromContext(c)
	if err != nil {
		return err
	}
	return spammer.Unstuck(config)
}
