package state_native_test

import (
	"context"
	"testing"

	"github.com/theQRL/go-zond/common/hexutil"
	statenative "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	"github.com/theQRL/qrysm/container/trie"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestBeaconStateMerkleProofs_capella(t *testing.T) {
	ctx := context.Background()
	capella, err := util.NewBeaconStateCapella()
	require.NoError(t, err)
	htr, err := capella.HashTreeRoot(ctx)
	require.NoError(t, err)
	results := []string{
		"0x6cf04127db05441cd833107a52be852868890e4317e6a02ab47683aa75964220",
		"0xe8facaa9be1c488207092f135ca6159f7998f313459b4198f46a9433f8b346e6",
		"0x0a7910590f2a08faa740a5c40e919722b80a786d18d146318309926a6b2ab95e",
		"0xedbd408e9bd85f6ecde880cc5854b32d22a684805128869056bf1ea404317eb3",
		"0x5f9bb608307c4f803bd4864fbe266fdd9b3453f169178b6d4205555f47f7af7a",
	}
	t.Run("current sync committee", func(t *testing.T) {
		cscp, err := capella.CurrentSyncCommitteeProof(ctx)
		require.NoError(t, err)
		require.Equal(t, len(cscp), 5)
		for i, bytes := range cscp {
			require.Equal(t, results[i], hexutil.Encode(bytes))
		}
	})
	t.Run("next sync committee", func(t *testing.T) {
		nscp, err := capella.NextSyncCommitteeProof(ctx)
		require.NoError(t, err)
		require.Equal(t, len(nscp), 5)
		for i, bytes := range nscp {
			require.Equal(t, results[i], hexutil.Encode(bytes))
		}
	})
	t.Run("finalized root", func(t *testing.T) {
		finalizedRoot := capella.FinalizedCheckpoint().Root
		proof, err := capella.FinalizedRootProof(ctx)
		require.NoError(t, err)
		gIndex := statenative.FinalizedRootGeneralizedIndex()
		valid := trie.VerifyMerkleProof(htr[:], finalizedRoot, gIndex, proof)
		require.Equal(t, true, valid)
	})
	t.Run("recomputes root on dirty fields", func(t *testing.T) {
		currentRoot, err := capella.HashTreeRoot(ctx)
		require.NoError(t, err)
		cpt := capella.FinalizedCheckpoint()
		require.NoError(t, err)

		// Edit the checkpoint.
		cpt.Epoch = 100
		require.NoError(t, capella.SetFinalizedCheckpoint(cpt))

		// Produce a proof for the finalized root.
		proof, err := capella.FinalizedRootProof(ctx)
		require.NoError(t, err)

		// We expect the previous step to have triggered
		// a recomputation of dirty fields in the beacon state, resulting
		// in a new hash tree root as the finalized checkpoint had previously
		// changed and should have been marked as a dirty state field.
		// The proof validity should be false for the old root, but true for the new.
		finalizedRoot := capella.FinalizedCheckpoint().Root
		gIndex := statenative.FinalizedRootGeneralizedIndex()
		valid := trie.VerifyMerkleProof(currentRoot[:], finalizedRoot, gIndex, proof)
		require.Equal(t, false, valid)

		newRoot, err := capella.HashTreeRoot(ctx)
		require.NoError(t, err)

		valid = trie.VerifyMerkleProof(newRoot[:], finalizedRoot, gIndex, proof)
		require.Equal(t, true, valid)
	})
}
