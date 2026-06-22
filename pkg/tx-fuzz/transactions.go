package txfuzz

import (
	"context"
	"math/big"
	"math/rand"

	"github.com/theQRL/go-qrl/common"
	"github.com/theQRL/go-qrl/core/types"
	"github.com/theQRL/go-qrl/params"
	"github.com/theQRL/go-qrl/qrlclient"
	"github.com/theQRL/go-qrl/rpc"
	"github.com/theQRL/qrysm/pkg/FuzzyVM/filler"
	"github.com/theQRL/qrysm/pkg/FuzzyVM/generator"
)

// RandomCode creates a random byte code from the passed filler.
func RandomCode(f *filler.Filler) []byte {
	_, code := generator.GenerateProgram(f)
	return code
}

// RandomTx creates a random transaction.
func RandomTx(f *filler.Filler) (*types.Transaction, error) {
	nonce := uint64(rand.Int63())
	gasFeeCap := big.NewInt(rand.Int63())
	gasTipCap := big.NewInt(rand.Int63())
	chainID := big.NewInt(rand.Int63())
	return RandomValidTx(nil, f, common.Address{}, nonce, gasFeeCap, gasTipCap, chainID, false)
}

type txConf struct {
	rpc       *rpc.Client
	nonce     uint64
	sender    common.Address
	to        *common.Address
	value     *big.Int
	gasLimit  uint64
	gasFeeCap *big.Int
	gasTipCap *big.Int
	chainID   *big.Int
	code      []byte
}

func initDefaultTxConf(rpc *rpc.Client, f *filler.Filler, sender common.Address, nonce uint64, gasFeeCap, gasTipCap, chainID *big.Int) *txConf {
	// Set fields if non-nil
	if rpc != nil {
		client := qrlclient.NewClient(rpc)
		var err error
		if gasFeeCap == nil {
			gasFeeCap, err = client.SuggestGasPrice(context.Background())
			if err != nil {
				gasFeeCap = big.NewInt(1)
			}
		}
		if gasTipCap == nil {
			gasTipCap, err = client.SuggestGasTipCap(context.Background())
			if err != nil {
				gasTipCap = big.NewInt(1)
			}
		}
		if chainID == nil {
			chainID, err = client.ChainID(context.Background())
			if err != nil {
				chainID = big.NewInt(1)
			}
		}
	}
	gas := uint64(100000)
	to := randomAddress()
	code := RandomCode(f)
	value := big.NewInt(0)
	if len(code) > 128 {
		code = code[:128]
	}
	return &txConf{
		rpc:       rpc,
		nonce:     nonce,
		sender:    sender,
		to:        &to,
		value:     value,
		gasLimit:  gas,
		gasFeeCap: gasFeeCap,
		gasTipCap: gasTipCap,
		chainID:   chainID,
		code:      code,
	}
}

// RandomValidTx creates a random valid transaction.
// It does not mean that the transaction will succeed, but that it is well-formed.
// If gasPrice is not set, we will try to get it from the rpc
// If chainID is not set, we will try to get it from the rpc
func RandomValidTx(rpc *rpc.Client, f *filler.Filler, sender common.Address, nonce uint64, gasFeeCap, gasTipCap, chainID *big.Int, al bool) (*types.Transaction, error) {
	conf := initDefaultTxConf(rpc, f, sender, nonce, gasFeeCap, gasTipCap, chainID)
	if al {
		index := rand.Intn(len(alStrategies))
		return alStrategies[index](conf)
	} else {
		index := rand.Intn(len(noAlStrategies))
		return noAlStrategies[index](conf)
	}
}

type txCreationStrategy func(conf *txConf) (*types.Transaction, error)

var noAlStrategies = []txCreationStrategy{
	contractCreation1559,
	tx1559,
}

var alStrategies = append(noAlStrategies, []txCreationStrategy{
	fullAl1559ContractCreation,
	fullAl1559Tx,
}...)

func contractCreation1559(conf *txConf) (*types.Transaction, error) {
	// 1559 contract creation
	tip, feecap, err := getCaps(conf.rpc, conf.gasFeeCap)
	if err != nil {
		return nil, err
	}
	return new1559Tx(conf.nonce, nil, conf.gasLimit, conf.chainID, tip, feecap, conf.value, conf.code, make(types.AccessList, 0)), nil
}

func tx1559(conf *txConf) (*types.Transaction, error) {
	// 1559 transaction
	tip, feecap, err := getCaps(conf.rpc, conf.gasFeeCap)
	if err != nil {
		return nil, err
	}
	return new1559Tx(conf.nonce, conf.to, conf.gasLimit, conf.chainID, tip, feecap, conf.value, conf.code, make(types.AccessList, 0)), nil
}

func fullAl1559ContractCreation(conf *txConf) (*types.Transaction, error) {
	// 1559 contract creation with AL
	tx := types.NewTx(&types.DynamicFeeTx{
		Nonce:     conf.nonce,
		Value:     conf.value,
		Gas:       conf.gasLimit,
		GasFeeCap: conf.gasFeeCap,
		GasTipCap: conf.gasTipCap,
		Data:      conf.code,
	})
	al, err := CreateAccessList(conf.rpc, tx, conf.sender)
	if err != nil {
		return nil, err
	}
	tip, feecap, err := getCaps(conf.rpc, conf.gasFeeCap)
	if err != nil {
		return nil, err
	}
	return new1559Tx(conf.nonce, nil, conf.gasLimit, conf.chainID, tip, feecap, conf.value, conf.code, *al), nil
}

func fullAl1559Tx(conf *txConf) (*types.Transaction, error) {
	// 1559 tx with AL
	tx := types.NewTx(&types.DynamicFeeTx{
		Nonce:     conf.nonce,
		To:        conf.to,
		Value:     conf.value,
		Gas:       conf.gasLimit,
		GasFeeCap: conf.gasFeeCap,
		GasTipCap: conf.gasTipCap,
		Data:      conf.code,
	})
	al, err := CreateAccessList(conf.rpc, tx, conf.sender)
	if err != nil {
		return nil, err
	}
	tip, feecap, err := getCaps(conf.rpc, conf.gasFeeCap)
	if err != nil {
		return nil, err
	}
	return new1559Tx(conf.nonce, conf.to, conf.gasLimit, conf.chainID, tip, feecap, conf.value, conf.code, *al), nil
}

func new1559Tx(nonce uint64, to *common.Address, gasLimit uint64, chainID, tip, feeCap, value *big.Int, code []byte, al types.AccessList) *types.Transaction {
	return types.NewTx(&types.DynamicFeeTx{
		ChainID:    chainID,
		Nonce:      nonce,
		GasTipCap:  tip,
		GasFeeCap:  feeCap,
		Gas:        gasLimit,
		To:         to,
		Value:      value,
		Data:       code,
		AccessList: al,
	})
}

func getCaps(rpc *rpc.Client, defaultGasFeeCap *big.Int) (*big.Int, *big.Int, error) {
	if rpc == nil {
		tip := new(big.Int).Mul(big.NewInt(1), big.NewInt(params.Shor))
		if defaultGasFeeCap.Cmp(tip) >= 0 {
			feeCap := new(big.Int).Sub(defaultGasFeeCap, tip)
			return tip, feeCap, nil
		}
		return big.NewInt(0), defaultGasFeeCap, nil
	}
	client := qrlclient.NewClient(rpc)
	tip, err := client.SuggestGasTipCap(context.Background())
	if err != nil {
		return nil, nil, err
	}
	feeCap, err := client.SuggestGasPrice(context.Background())
	return tip, feeCap, err
}
