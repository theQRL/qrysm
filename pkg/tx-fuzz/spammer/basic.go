package spammer

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/theQRL/go-qrl/accounts/abi/bind"
	"github.com/theQRL/go-qrl/core/types"
	"github.com/theQRL/go-qrl/crypto/pqcrypto/wallet"
	"github.com/theQRL/go-qrl/log"
	"github.com/theQRL/go-qrl/qrlclient"
	"github.com/theQRL/qrysm/pkg/FuzzyVM/filler"
	txfuzz "github.com/theQRL/qrysm/pkg/tx-fuzz"
)

const TX_TIMEOUT = 5 * time.Minute

func SendBasicTransactions(config *Config, w wallet.Wallet, f *filler.Filler) error {
	backend := qrlclient.NewClient(config.backend)
	sender := w.GetAddress()
	chainID, err := backend.ChainID(context.Background())
	if err != nil {
		log.Warn("Could not get chainID, using default")
		chainID = big.NewInt(0x01000666)
	}

	var lastTx *types.Transaction
	for i := uint64(0); i < config.N; i++ {
		nonce, err := backend.NonceAt(context.Background(), sender, big.NewInt(-1))
		if err != nil {
			return err
		}
		tx, err := txfuzz.RandomValidTx(config.backend, f, sender, nonce, nil, nil, nil, config.accessList)
		if err != nil {
			log.Warn("Could not create valid tx: %v", nonce)
			return err
		}
		signedTx, err := types.SignTx(tx, types.NewZondSigner(chainID), w)
		if err != nil {
			return err
		}
		if err := backend.SendTransaction(context.Background(), signedTx); err != nil {
			log.Warn("Could not submit transaction: %v", err)
			return err
		}
		lastTx = signedTx
		time.Sleep(10 * time.Millisecond)
	}
	if lastTx != nil {
		ctx, cancel := context.WithTimeout(context.Background(), TX_TIMEOUT)
		defer cancel()
		if _, err := bind.WaitMined(ctx, backend, lastTx); err != nil {
			fmt.Printf("Waiting for transactions to be mined failed: %v\n", err.Error())
		}
	}
	return nil
}
