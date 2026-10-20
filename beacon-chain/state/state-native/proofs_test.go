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
		"0xcd3e218d06d5731ab993f09346d9f7b8d1931dd2b6369003a992edf4208b73f5",
	}
	t.Run("current sync committee", func(t *testing.T) {
		cscp, err := zond.CurrentSyncCommitteeProof(ctx)
		require.NoError(t, err)
		require.Equal(t, len(cscp), 5)
		for i, bytes := range cscp {
			require.Equal(t, results[i], hexutil.Encode(bytes))
		}
		committee, err := zond.CurrentSyncCommittee()
		require.NoError(t, err)
		position, err := zond.(*statenative.BeaconState).CurrentSyncCommitteeGeneralizedIndex()
		require.NoError(t, err)
		requireProofVerifies(t, htr[:], committee, position, cscp)
	})
	t.Run("next sync committee", func(t *testing.T) {
		nscp, err := zond.NextSyncCommitteeProof(ctx)
		require.NoError(t, err)
		require.Equal(t, len(nscp), 5)
		for i, bytes := range nscp {
			require.Equal(t, results[i], hexutil.Encode(bytes))
		}
		committee, err := zond.NextSyncCommittee()
		require.NoError(t, err)
		position, err := zond.(*statenative.BeaconState).NextSyncCommitteeGeneralizedIndex()
		require.NoError(t, err)
		requireProofVerifies(t, htr[:], committee, position, nscp)
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

// requireProofVerifies checks a top-level state field proof against the state
// root, so the expected branch values above are anchored to the root rather
// than only to a previous run. The state tree has depth 5 (len(proof)), so the
// generalized index of the field at position p is 2^5 + p.
func requireProofVerifies(t *testing.T, stateRoot []byte, field interface {
	HashTreeRoot() ([32]byte, error)
}, position uint64, proof [][]byte) {
	leaf, err := field.HashTreeRoot()
	require.NoError(t, err)
	gIndex := uint64(1)<<uint64(len(proof)) + position
	require.Equal(t, true, trie.VerifyMerkleProof(stateRoot, leaf[:], gIndex, proof))
}
