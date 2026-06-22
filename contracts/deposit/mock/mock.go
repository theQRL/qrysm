package mock

import (
	"math/big"
	"strings"

	"github.com/theQRL/go-qrl/accounts/abi"
	"github.com/theQRL/go-qrl/accounts/abi/bind"
	"github.com/theQRL/go-qrl/accounts/abi/bind/backends"
	"github.com/theQRL/go-qrl/common"
	"github.com/theQRL/go-qrl/core"
	"github.com/theQRL/go-qrl/core/types"
	"github.com/theQRL/go-qrl/crypto/pqcrypto/wallet"
	"github.com/theQRL/qrysm/contracts/deposit"
)

var (
	amount40000Quanta     = "40000000000000000000000"
	amountLessThan1Quanta = "500000000000000000"
)

// TestAccount represents a test account in the simulated backend,
// through which we can perform actions on the execution chain.
type TestAccount struct {
	Addr         common.Address
	ContractAddr common.Address
	Contract     *deposit.DepositContract
	Backend      *backends.SimulatedBackend
	TxOpts       *bind.TransactOpts
}

// Setup creates the simulated backend with the deposit contract deployed
func Setup() (*TestAccount, error) {
	genesis := make(core.GenesisAlloc)
	wallet, err := wallet.Generate(wallet.ML_DSA_87)
	if err != nil {
		return nil, err
	}

	addr := wallet.GetAddress()
	txOpts, err := bind.NewKeyedTransactorWithChainID(wallet, big.NewInt(1337))
	if err != nil {
		return nil, err
	}
	startingBalance, _ := new(big.Int).SetString("100000000000000000000000000000000000000", 10)
	genesis[addr] = core.GenesisAccount{Balance: startingBalance}
	backend := backends.NewSimulatedBackend(genesis, 20000000)

	contractAddr, _, contract, err := DeployDepositContract(txOpts, backend)
	if err != nil {
		return nil, err
	}
	backend.Commit()

	return &TestAccount{addr, contractAddr, contract, backend, txOpts}, nil
}

// Amount40000Quanta returns 40000Quanta(in planck) in terms of the big.Int type.
func Amount40000Quanta() *big.Int {
	amount, _ := new(big.Int).SetString(amount40000Quanta, 10)
	return amount
}

// LessThan1Quanta returns less than 1 Quanta(in planck) in terms of the big.Int type.
func LessThan1Quanta() *big.Int {
	amount, _ := new(big.Int).SetString(amountLessThan1Quanta, 10)
	return amount
}

// DeployDepositContract deploys a new QRL contract, binding an instance of DepositContract to it.
func DeployDepositContract(auth *bind.TransactOpts, backend bind.ContractBackend) (common.Address, *types.Transaction, *deposit.DepositContract, error) {
	parsed, err := abi.JSON(strings.NewReader(deposit.DepositContractABI))
	if err != nil {
		return common.Address{}, nil, nil, err
	}

	address, tx, contract, err := bind.DeployContract(auth, parsed, common.FromHex(deposit.DepositContractCreationCodeHex()), backend)
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	return address, tx, &deposit.DepositContract{
		DepositContractCaller:     deposit.NewDepositContractCallerFromBoundContract(contract),
		DepositContractTransactor: deposit.NewDepositContractTransactorFromBoundContract(contract),
		DepositContractFilterer:   deposit.NewDepositContractFiltererFromBoundContract(contract),
	}, nil
}
