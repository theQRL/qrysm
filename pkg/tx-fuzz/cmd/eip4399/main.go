package main

import (
	"context"
	"fmt"

	"github.com/theQRL/go-qrl/common"
	"github.com/theQRL/go-qrl/core/types"
	"github.com/theQRL/go-qrl/crypto/pqcrypto/wallet"
	"github.com/theQRL/go-qrl/qrlclient"
	"github.com/theQRL/go-qrl/rpc"
	txfuzz "github.com/theQRL/qrysm/pkg/tx-fuzz"
)

var (
	address = "http://127.0.0.1:8545"
)

func main() {
	cl, wallet := getRealBackend()
	backend := qrlclient.NewClient(cl)
	sender, err := common.NewAddressFromString(txfuzz.ADDR)
	if err != nil {
		panic(err)
	}
	nonce, err := backend.PendingNonceAt(context.Background(), sender)
	if err != nil {
		panic(err)
	}
	chainid, err := backend.ChainID(context.Background())
	if err != nil {
		panic(err)
	}
	fmt.Printf("Nonce: %v\n", nonce)

	gasTipCap, _ := backend.SuggestGasTipCap(context.Background())
	gasFeeCap, _ := backend.SuggestGasPrice(context.Background())
	tx := types.NewTx(&types.DynamicFeeTx{
		Nonce:     nonce,
		Value:     common.Big1,
		Gas:       500000,
		GasFeeCap: gasFeeCap,
		GasTipCap: gasTipCap,
		Data:      []byte{0x44, 0x44, 0x55},
	})
	signedTx, _ := types.SignTx(tx, types.NewZondSigner(chainid), wallet)
	backend.SendTransaction(context.Background(), signedTx)
}

func getRealBackend() (*rpc.Client, wallet.Wallet) {
	// qrl.sendTransaction({from:personal.listAccounts[0], to:"Q00000000000000000000000000000000f1c4b3a519fb7cbb0c95143d22234411932151b9cc98b510d34bebb6f99f37abc1051bacd1853dc25bf92ead382cbaf7", value: "100000000000000"})
	acc, err := wallet.RestoreFromSeedHex(txfuzz.SEED)
	if err != nil {
		panic(err)
	}
	if addr := common.Address(acc.GetAddress()); addr.Hex() != txfuzz.ADDR {
		panic(fmt.Sprintf("wrong address want %s got %s", addr.Hex(), txfuzz.ADDR))
	}

	cl, err := rpc.Dial(address)
	if err != nil {
		panic(err)
	}
	return cl, acc
}
