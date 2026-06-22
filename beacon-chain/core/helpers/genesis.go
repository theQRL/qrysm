package helpers

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/container/trie"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

// UpdateGenesisExecutionData updates execution data for genesis state.
func UpdateGenesisExecutionData(state state.BeaconState, deposits []*qrysmpb.Deposit, executionData *qrysmpb.ExecutionData) (state.BeaconState, error) {
	if executionData == nil {
		return nil, errors.New("no executionData provided for genesis state")
	}

	leaves := make([][]byte, 0, len(deposits))
	for _, deposit := range deposits {
		if deposit == nil || deposit.Data == nil {
			return nil, fmt.Errorf("nil deposit or deposit with nil data cannot be processed: %v", deposit)
		}
		hash, err := deposit.Data.HashTreeRoot()
		if err != nil {
			return nil, err
		}
		leaves = append(leaves, hash[:])
	}
	var t *trie.SparseMerkleTrie
	var err error
	if len(leaves) > 0 {
		t, err = trie.GenerateTrieFromItems(leaves, params.BeaconConfig().DepositContractTreeDepth)
		if err != nil {
			return nil, err
		}
	} else {
		t, err = trie.NewTrie(params.BeaconConfig().DepositContractTreeDepth)
		if err != nil {
			return nil, err
		}
	}

	depositRoot, err := t.HashTreeRoot()
	if err != nil {
		return nil, err
	}
	executionData.DepositRoot = depositRoot[:]
	err = state.SetExecutionData(executionData)
	if err != nil {
		return nil, err
	}
	return state, nil
}
