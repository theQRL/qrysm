package interop

import (
	"bytes"
	"context"
	"math/big"
	"testing"

	"github.com/theQRL/go-qrl/accounts/abi/bind"
	"github.com/theQRL/go-qrl/accounts/abi/bind/backends"
	"github.com/theQRL/go-qrl/common"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/container/trie"
	contracts "github.com/theQRL/qrysm/contracts/deposit"
	depositmock "github.com/theQRL/qrysm/contracts/deposit/mock"
	"github.com/theQRL/qrysm/testing/require"
)

// A genesis allocation does not run the deposit contract's constructor: GqrlTestnetGenesis
// installs runtime code derived from the creation bytecode and the storage listed in
// DefaultDepositContractStorage. Both depend on the compiler's output, so this test compares
// them with a contract created by its constructor.
func TestGqrlTestnetGenesis_DepositContractMatchesConstructor(t *testing.T) {
	ctx := context.Background()
	cfg := params.BeaconConfig()

	deployed, err := depositmock.Setup()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, deployed.Backend.Close())
	})

	addr, err := common.NewAddressFromString(cfg.DepositContractAddress)
	require.NoError(t, err)
	alloc := GqrlTestnetGenesis(0, cfg).Alloc
	account, ok := alloc[addr]
	require.Equal(t, true, ok, "Deposit contract is missing from the genesis allocation")

	deployedCode, err := deployed.Backend.CodeAt(ctx, deployed.ContractAddr, nil)
	require.NoError(t, err)
	require.Equal(t, true, bytes.Equal(deployedCode, account.Code),
		"Genesis code (%d bytes) is not the runtime code the constructor installs (%d bytes)", len(account.Code), len(deployedCode))

	// branch, deposit_count and zero_hashes take slots 0x00 to 0x20. The range also covers the
	// slots of the Ethereum layout, up to 0x40, so that values left there are reported as well.
	// This scan only names the slot that differs; the storage roots compared below cover every
	// slot.
	const lastSlot = 0x40
	for slot := int64(0); slot <= lastSlot; slot++ {
		key := common.BigToHash(big.NewInt(slot))
		want, err := deployed.Backend.StorageAt(ctx, deployed.ContractAddr, key, nil)
		require.NoError(t, err)
		require.Equal(t, common.BytesToStorageValue64(want), account.Storage[key],
			"Genesis storage differs from the constructor's at slot %#x", slot)
	}

	backend := backends.NewSimulatedBackend(alloc, 20000000)
	t.Cleanup(func() {
		require.NoError(t, backend.Close())
	})

	// The storage trie root covers all of the contract's storage, whichever slots it uses.
	deployedState, err := deployed.Backend.Blockchain().State()
	require.NoError(t, err)
	genesisState, err := backend.Blockchain().State()
	require.NoError(t, err)
	require.Equal(t, deployedState.GetStorageRoot(deployed.ContractAddr), genesisState.GetStorageRoot(addr),
		"Genesis storage is not the storage the constructor leaves, in a slot above %#x", lastSlot)

	// The root of the empty tree depends on every zero hash and on the deposit count.
	contract, err := contracts.NewDepositContract(addr, backend)
	require.NoError(t, err)
	root, err := contract.GetDepositRoot(&bind.CallOpts{})
	require.NoError(t, err)

	localTrie, err := trie.NewTrie(cfg.DepositContractTreeDepth)
	require.NoError(t, err)
	localRoot, err := localTrie.HashTreeRoot()
	require.NoError(t, err)
	require.Equal(t, localRoot, root, "Deposit root of the genesis contract and of the empty local deposit trie are not equal")
}
