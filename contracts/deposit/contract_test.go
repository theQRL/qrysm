package deposit_test

import (
	"context"
	"encoding/binary"
	"math/big"
	"testing"

	qrl "github.com/theQRL/go-qrl"
	"github.com/theQRL/go-qrl/common"
	"github.com/theQRL/qrysm/config/params"
	depositcontract "github.com/theQRL/qrysm/contracts/deposit"
	"github.com/theQRL/qrysm/contracts/deposit/mock"
	"github.com/theQRL/qrysm/runtime/interop"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

func TestSetupRegistrationContract_OK(t *testing.T) {
	_, err := mock.Setup()
	assert.NoError(t, err, "Can not deploy validator registration contract")
}

// negative test case, deposit with less than 1 Quanta which is less than the top off amount.
func TestRegister_Below1Quanta(t *testing.T) {
	testAccount, err := mock.Setup()
	require.NoError(t, err)

	// Generate deposit data
	privKeys, pubKeys, err := interop.DeterministicallyGenerateKeys(0 /*startIndex*/, 1)
	require.NoError(t, err)
	depositDataItems, depositDataRoots, err := interop.DepositDataFromKeys(privKeys, pubKeys)
	require.NoError(t, err)

	var depositDataRoot [32]byte
	copy(depositDataRoot[:], depositDataRoots[0])
	testAccount.TxOpts.Value = mock.LessThan1Quanta()
	_, err = testAccount.Contract.Deposit(testAccount.TxOpts, pubKeys[0].Marshal(), depositDataItems[0].WithdrawalRecipient, depositDataItems[0].RandaoCommitment, depositDataItems[0].Signature, depositDataRoot)
	assert.ErrorContains(t, "execution reverted", err, "Validator registration should have failed with insufficient deposit")
}

// normal test case, test depositing 40000 Quanta and verify HashChainValue event is correctly emitted.
func TestValidatorRegister_OK(t *testing.T) {
	testAccount, err := mock.Setup()
	require.NoError(t, err)
	testAccount.TxOpts.Value = mock.Amount40000Quanta()

	// Generate deposit data
	privKeys, pubKeys, err := interop.DeterministicallyGenerateKeys(0 /*startIndex*/, 1)
	require.NoError(t, err)
	depositDataItems, depositDataRoots, err := interop.DepositDataFromKeys(privKeys, pubKeys)
	require.NoError(t, err)

	var depositDataRoot [32]byte
	copy(depositDataRoot[:], depositDataRoots[0])
	_, err = testAccount.Contract.Deposit(testAccount.TxOpts, pubKeys[0].Marshal(), depositDataItems[0].WithdrawalRecipient, depositDataItems[0].RandaoCommitment, depositDataItems[0].Signature, depositDataRoot)
	testAccount.Backend.Commit()
	require.NoError(t, err, "Validator registration failed")
	_, err = testAccount.Contract.Deposit(testAccount.TxOpts, pubKeys[0].Marshal(), depositDataItems[0].WithdrawalRecipient, depositDataItems[0].RandaoCommitment, depositDataItems[0].Signature, depositDataRoot)
	testAccount.Backend.Commit()
	assert.NoError(t, err, "Validator registration failed")
	_, err = testAccount.Contract.Deposit(testAccount.TxOpts, pubKeys[0].Marshal(), depositDataItems[0].WithdrawalRecipient, depositDataItems[0].RandaoCommitment, depositDataItems[0].Signature, depositDataRoot)
	testAccount.Backend.Commit()
	assert.NoError(t, err, "Validator registration failed")

	query := qrl.FilterQuery{
		Addresses: []common.Address{
			testAccount.ContractAddr,
		},
	}

	logs, err := testAccount.Backend.FilterLogs(context.Background(), query)
	assert.NoError(t, err, "Unable to get logs of deposit contract")

	merkleTreeIndex := make([]uint64, 5)

	for i, log := range logs {
		_, _, _, _, _, idx, err := depositcontract.UnpackDepositLogData(log.Data)
		require.NoError(t, err, "Unable to unpack log data")
		merkleTreeIndex[i] = binary.LittleEndian.Uint64(idx)
	}

	assert.Equal(t, uint64(0), merkleTreeIndex[0], "Deposit event total deposit count mismatched")
	assert.Equal(t, uint64(1), merkleTreeIndex[1], "Deposit event total deposit count mismatched")
	assert.Equal(t, uint64(2), merkleTreeIndex[2], "Deposit event total deposit count mismatched")
}

// The contract floor is MIN_DEPOSIT_AMOUNT (2000 QRL): one shor below it
// reverts and exactly it is accepted. The deposit data root is signed over the
// actual amount, so the accepted case builds its own deposit input. The mock
// amount is also pinned to the config value so the three cannot drift apart.
func TestRegister_MinimumDepositBoundary(t *testing.T) {
	testAccount, err := mock.Setup()
	require.NoError(t, err)

	privKeys, pubKeys, err := interop.DeterministicallyGenerateKeys(0 /*startIndex*/, 1)
	require.NoError(t, err)
	depositDataItems, _, err := interop.DepositDataFromKeys(privKeys, pubKeys)
	require.NoError(t, err)

	minAmount := params.BeaconConfig().MinDepositAmount
	wantPlanck := new(big.Int).Mul(new(big.Int).SetUint64(minAmount), big.NewInt(1e9))
	require.Equal(t, 0, wantPlanck.Cmp(mock.AmountMinimumQuanta()), "mock minimum must equal MinDepositAmount")
	withdrawalAddr := common.BytesToAddress(depositDataItems[0].WithdrawalRecipient)

	// Sign the root over the exact below-minimum amount so the floor is the only
	// reason the contract can revert; a mismatched root would also revert and
	// would hide a missing floor check.
	below, belowRoot, err := depositcontract.DepositInput(privKeys[0], withdrawalAddr, minAmount-1, params.BeaconConfig().GenesisForkVersion)
	require.NoError(t, err)
	testAccount.TxOpts.Value = mock.AmountBelowMinimumQuanta()
	_, err = testAccount.Contract.Deposit(testAccount.TxOpts, below.PublicKey, below.WithdrawalRecipient, below.RandaoCommitment, below.Signature, belowRoot)
	assert.ErrorContains(t, "DepositContract: deposit value too low", err, "deposit one shor below MIN_DEPOSIT_AMOUNT should revert on the floor")

	data, root, err := depositcontract.DepositInput(privKeys[0], withdrawalAddr, minAmount, params.BeaconConfig().GenesisForkVersion)
	require.NoError(t, err)
	testAccount.TxOpts.Value = mock.AmountMinimumQuanta()
	_, err = testAccount.Contract.Deposit(testAccount.TxOpts, data.PublicKey, data.WithdrawalRecipient, data.RandaoCommitment, data.Signature, root)
	testAccount.Backend.Commit()
	require.NoError(t, err, "deposit of exactly MIN_DEPOSIT_AMOUNT should be accepted")
}
