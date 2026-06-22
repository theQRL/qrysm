// Package interop contains deterministic utilities for generating
// genesis states and keys.
package interop

import (
	"context"

	"github.com/pkg/errors"
	coreState "github.com/theQRL/qrysm/beacon-chain/core/transition"
	statenative "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/container/trie"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/time"
)

// GenerateGenesisStateZond deterministically given a genesis time and number of validators.
// If a genesis time of 0 is supplied it is set to the current time.
func GenerateGenesisStateZond(ctx context.Context, genesisTime, numValidators uint64, ep *enginev1.ExecutionPayloadZond, ed *qrysmpb.ExecutionData) (*qrysmpb.BeaconStateZond, []*qrysmpb.Deposit, error) {
	privKeys, pubKeys, err := DeterministicallyGenerateKeys(0 /*startIndex*/, numValidators)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "could not deterministically generate keys for %d validators", numValidators)
	}
	depositDataItems, depositDataRoots, err := DepositDataFromKeys(privKeys, pubKeys)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not generate deposit data from keys")
	}
	return GenerateGenesisStateZondFromDepositData(ctx, genesisTime, depositDataItems, depositDataRoots, ep, ed)
}

// GenerateGenesisStateZondFromDepositData creates a genesis state given a list of
// deposit data items and their corresponding roots.
func GenerateGenesisStateZondFromDepositData(
	ctx context.Context, genesisTime uint64, depositData []*qrysmpb.Deposit_Data, depositDataRoots [][]byte, ep *enginev1.ExecutionPayloadZond, e1d *qrysmpb.ExecutionData,
) (*qrysmpb.BeaconStateZond, []*qrysmpb.Deposit, error) {
	t, err := trie.GenerateTrieFromItems(depositDataRoots, params.BeaconConfig().DepositContractTreeDepth)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not generate Merkle trie for deposit proofs")
	}
	deposits, err := GenerateDepositsFromData(depositData, t)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not generate deposits from the deposit data provided")
	}
	if genesisTime == 0 {
		genesisTime = uint64(time.Now().Unix())
	}
	beaconState, err := coreState.GenesisBeaconStateZond(ctx, deposits, genesisTime, e1d, ep)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not generate genesis state")
	}
	bsi := beaconState.ToProtoUnsafe()
	pbc, ok := bsi.(*qrysmpb.BeaconStateZond)
	if !ok {
		return nil, nil, errors.New("unexpected BeaconState version")
	}
	pbState, err := statenative.ProtobufBeaconStateZond(pbc)
	if err != nil {
		return nil, nil, err
	}
	return pbState, deposits, nil
}
