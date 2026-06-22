package spammer

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/theQRL/go-qrl/accounts/abi/bind"
	"github.com/theQRL/go-qrl/common"
	"github.com/theQRL/go-qrl/core/types"
	"github.com/theQRL/go-qrl/crypto/pqcrypto/wallet"
	"github.com/theQRL/go-qrl/qrlclient"
)

const batchSize = 50

func SendTx(w wallet.Wallet, backend *qrlclient.Client, to common.Address, value *big.Int) (*types.Transaction, error) {
	sender := w.GetAddress()
	nonce, err := backend.NonceAt(context.Background(), sender, nil)
	if err != nil {
		fmt.Printf("Could not get pending nonce: %v", err)
	}
	return sendTxWithNonce(w, backend, to, value, nonce)
}

func sendTxWithNonce(w wallet.Wallet, backend *qrlclient.Client, to common.Address, value *big.Int, nonce uint64) (*types.Transaction, error) {
	chainid, err := backend.ChainID(context.Background())
	if err != nil {
		return nil, err
	}
	gasFeeCap, _ := backend.SuggestGasPrice(context.Background())
	gasTipCap, _ := backend.SuggestGasTipCap(context.Background())
	tx := types.NewTx(&types.DynamicFeeTx{
		Nonce:     nonce,
		To:        &to,
		Value:     value,
		Gas:       500000,
		GasFeeCap: gasFeeCap,
		GasTipCap: gasTipCap,
		Data:      nil,
	})
	signedTx, _ := types.SignTx(tx, types.NewZondSigner(chainid), w)
	return signedTx, backend.SendTransaction(context.Background(), signedTx)
}

func sendRecurringTx(w wallet.Wallet, backend *qrlclient.Client, to common.Address, value *big.Int, numTxs uint64) (*types.Transaction, error) {
	sender := w.GetAddress()
	nonce, err := backend.NonceAt(context.Background(), sender, nil)
	if err != nil {
		return nil, err
	}
	var (
		tx *types.Transaction
	)
	for i := 0; i < int(numTxs); i++ {
		tx, err = sendTxWithNonce(w, backend, to, value, nonce+uint64(i))
	}
	return tx, err
}

func Unstuck(config *Config) error {
	if err := tryUnstuck(config, config.faucetAcc); err != nil {
		return err
	}
	for _, acc := range config.accs {
		if err := tryUnstuck(config, acc); err != nil {
			return err
		}
	}
	return nil
}

func tryUnstuck(config *Config, w wallet.Wallet) error {
	var (
		client = qrlclient.NewClient(config.backend)
		addr   = w.GetAddress()
	)
	for range 100 {
		noTx, err := isStuck(config, addr)
		if err != nil {
			return err
		}
		if noTx == 0 {
			return nil
		}

		// Self-transfer of 1 planck to unstuck
		if noTx > batchSize {
			noTx = batchSize
		}
		fmt.Println("Sending transaction to unstuck account")
		tx, err := sendRecurringTx(w, client, addr, big.NewInt(1), noTx)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
		defer cancel()
		if _, err := bind.WaitMined(ctx, client, tx); err != nil {
			return err
		}
	}
	fmt.Printf("Could not unstuck account %v after 100 tries\n", addr)
	return errors.New("unstuck timed out, please retry manually")
}

func isStuck(config *Config, account common.Address) (uint64, error) {
	client := qrlclient.NewClient(config.backend)
	nonce, err := client.NonceAt(context.Background(), account, nil)
	if err != nil {
		return 0, err
	}

	pendingNonce, err := client.PendingNonceAt(context.Background(), account)
	if err != nil {
		return 0, err
	}

	if pendingNonce != nonce {
		fmt.Printf("Account %v is stuck: pendingNonce: %v currentNonce: %v, missing nonces: %v\n", account, pendingNonce, nonce, pendingNonce-nonce)
		return pendingNonce - nonce, nil
	}
	return 0, nil
}
