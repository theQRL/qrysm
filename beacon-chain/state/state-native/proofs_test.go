package state_native_test

import (
	"context"
	"testing"

	"github.com/theQRL/go-qrl/common/hexutil"
	statenative "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	"github.com/theQRL/qrysm/container/trie"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestBeaconStateMerkleProofs_zond(t *testing.T) {
	ctx := context.Background()
	zond, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	htr, err := zond.HashTreeRoot(ctx)
	require.NoError(t, err)
	results := []string{
		"0xb58d900f5e182e3c50ef74969ea16c7726c549757cc23523c369587da7293784",
		"0xe8facaa9be1c488207092f135ca6159f7998f313459b4198f46a9433f8b346e6",
		"0x0a7910590f2a08faa740a5c40e919722b80a786d18d146318309926a6b2ab95e",
		"0x9fce5ce890405247edf65d7b4da2ad63f3de42ffc0da863c5176b389e38db34c",
		"0x00665d3d98a46ded2cec4c53541fe88f01f09da395395f7d40e393bb74d89e8f",
	}
	t.Run("current sync committee", func(t *testing.T) {
		cscp, err := zond.CurrentSyncCommitteeProof(ctx)
		require.NoError(t, err)
		require.Equal(t, len(cscp), 5)
		for i, bytes := range cscp {
			require.Equal(t, results[i], hexutil.Encode(bytes))
		}
	})
	t.Run("next sync committee", func(t *testing.T) {
		nscp, err := zond.NextSyncCommitteeProof(ctx)
		require.NoError(t, err)
		require.Equal(t, len(nscp), 5)
		for i, bytes := range nscp {
			require.Equal(t, results[i], hexutil.Encode(bytes))
		}
	})
	t.Run("finalized root", func(t *testing.T) {
		finalizedRoot := zond.FinalizedCheckpoint().Root
		proof, err := zond.FinalizedRootProof(ctx)
		require.NoError(t, err)
		gIndex := statenative.FinalizedRootGeneralizedIndex()
		valid := trie.VerifyMerkleProof(htr[:], finalizedRoot, gIndex, proof)
		require.Equal(t, true, valid)
	})
	t.Run("recomputes root on dirty fields", func(t *testing.T) {
		currentRoot, err := zond.HashTreeRoot(ctx)
		require.NoError(t, err)
		cpt := zond.FinalizedCheckpoint()
		require.NoError(t, err)

		// Edit the checkpoint.
		cpt.Epoch = 100
		require.NoError(t, zond.SetFinalizedCheckpoint(cpt))

		// Produce a proof for the finalized root.
		proof, err := zond.FinalizedRootProof(ctx)
		require.NoError(t, err)

		// We expect the previous step to have triggered
		// a recomputation of dirty fields in the beacon state, resulting
		// in a new hash tree root as the finalized checkpoint had previously
		// changed and should have been marked as a dirty state field.
		// The proof validity should be false for the old root, but true for the new.
		finalizedRoot := zond.FinalizedCheckpoint().Root
		gIndex := statenative.FinalizedRootGeneralizedIndex()
		valid := trie.VerifyMerkleProof(currentRoot[:], finalizedRoot, gIndex, proof)
		require.Equal(t, false, valid)

		newRoot, err := zond.HashTreeRoot(ctx)
		require.NoError(t, err)

		valid = trie.VerifyMerkleProof(newRoot[:], finalizedRoot, gIndex, proof)
		require.Equal(t, true, valid)
	})
}
