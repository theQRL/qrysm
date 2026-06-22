package synccommittee

import (
	"testing"

	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
)

func TestSyncCommitteeContributionCache_Nil(t *testing.T) {
	store := NewStore()
	require.Equal(t, errNilContribution, store.SaveSyncCommitteeContribution(nil))
}

func TestSyncCommitteeContributionCache_RoundTrip(t *testing.T) {
	store := NewStore()

	conts := []*qrysmpb.SyncCommitteeContribution{
		{Slot: 1, SubcommitteeIndex: 0, Signatures: [][]byte{{'a'}}},
		{Slot: 1, SubcommitteeIndex: 1, Signatures: [][]byte{{'b'}}},
		{Slot: 2, SubcommitteeIndex: 0, Signatures: [][]byte{{'c'}}},
		{Slot: 2, SubcommitteeIndex: 1, Signatures: [][]byte{{'d'}}},
		{Slot: 3, SubcommitteeIndex: 0, Signatures: [][]byte{{'e'}}},
		{Slot: 3, SubcommitteeIndex: 1, Signatures: [][]byte{{'f'}}},
		{Slot: 4, SubcommitteeIndex: 0, Signatures: [][]byte{{'g'}}},
		{Slot: 4, SubcommitteeIndex: 1, Signatures: [][]byte{{'h'}}},
		{Slot: 5, SubcommitteeIndex: 0, Signatures: [][]byte{{'i'}}},
		{Slot: 5, SubcommitteeIndex: 1, Signatures: [][]byte{{'j'}}},
		{Slot: 6, SubcommitteeIndex: 0, Signatures: [][]byte{{'k'}}},
		{Slot: 6, SubcommitteeIndex: 1, Signatures: [][]byte{{'l'}}},
	}

	for _, sig := range conts {
		require.NoError(t, store.SaveSyncCommitteeContribution(sig))
	}

	conts, err := store.SyncCommitteeContributions(1)
	require.NoError(t, err)
	require.DeepSSZEqual(t, []*qrysmpb.SyncCommitteeContribution{}, conts)

	conts, err = store.SyncCommitteeContributions(2)
	require.NoError(t, err)
	require.DeepSSZEqual(t, []*qrysmpb.SyncCommitteeContribution{}, conts)

	conts, err = store.SyncCommitteeContributions(3)
	require.NoError(t, err)
	require.DeepSSZEqual(t, []*qrysmpb.SyncCommitteeContribution{
		{Slot: 3, SubcommitteeIndex: 0, Signatures: [][]byte{{'e'}}},
		{Slot: 3, SubcommitteeIndex: 1, Signatures: [][]byte{{'f'}}},
	}, conts)

	conts, err = store.SyncCommitteeContributions(4)
	require.NoError(t, err)
	require.DeepSSZEqual(t, []*qrysmpb.SyncCommitteeContribution{
		{Slot: 4, SubcommitteeIndex: 0, Signatures: [][]byte{{'g'}}},
		{Slot: 4, SubcommitteeIndex: 1, Signatures: [][]byte{{'h'}}},
	}, conts)

	conts, err = store.SyncCommitteeContributions(5)
	require.NoError(t, err)
	require.DeepSSZEqual(t, []*qrysmpb.SyncCommitteeContribution{
		{Slot: 5, SubcommitteeIndex: 0, Signatures: [][]byte{{'i'}}},
		{Slot: 5, SubcommitteeIndex: 1, Signatures: [][]byte{{'j'}}},
	}, conts)

	conts, err = store.SyncCommitteeContributions(6)
	require.NoError(t, err)
	require.DeepSSZEqual(t, []*qrysmpb.SyncCommitteeContribution{
		{Slot: 6, SubcommitteeIndex: 0, Signatures: [][]byte{{'k'}}},
		{Slot: 6, SubcommitteeIndex: 1, Signatures: [][]byte{{'l'}}},
	}, conts)

	// All the contributions should persist after get.
	conts, err = store.SyncCommitteeContributions(1)
	require.NoError(t, err)
	require.DeepSSZEqual(t, []*qrysmpb.SyncCommitteeContribution{}, conts)
	conts, err = store.SyncCommitteeContributions(2)
	require.NoError(t, err)
	require.DeepSSZEqual(t, []*qrysmpb.SyncCommitteeContribution{}, conts)

	conts, err = store.SyncCommitteeContributions(3)
	require.NoError(t, err)
	require.DeepSSZEqual(t, []*qrysmpb.SyncCommitteeContribution{
		{Slot: 3, SubcommitteeIndex: 0, Signatures: [][]byte{{'e'}}},
		{Slot: 3, SubcommitteeIndex: 1, Signatures: [][]byte{{'f'}}},
	}, conts)

	conts, err = store.SyncCommitteeContributions(4)
	require.NoError(t, err)
	require.DeepSSZEqual(t, []*qrysmpb.SyncCommitteeContribution{
		{Slot: 4, SubcommitteeIndex: 0, Signatures: [][]byte{{'g'}}},
		{Slot: 4, SubcommitteeIndex: 1, Signatures: [][]byte{{'h'}}},
	}, conts)

	conts, err = store.SyncCommitteeContributions(5)
	require.NoError(t, err)
	require.DeepSSZEqual(t, []*qrysmpb.SyncCommitteeContribution{
		{Slot: 5, SubcommitteeIndex: 0, Signatures: [][]byte{{'i'}}},
		{Slot: 5, SubcommitteeIndex: 1, Signatures: [][]byte{{'j'}}},
	}, conts)

	conts, err = store.SyncCommitteeContributions(6)
	require.NoError(t, err)
	require.DeepSSZEqual(t, []*qrysmpb.SyncCommitteeContribution{
		{Slot: 6, SubcommitteeIndex: 0, Signatures: [][]byte{{'k'}}},
		{Slot: 6, SubcommitteeIndex: 1, Signatures: [][]byte{{'l'}}},
	}, conts)
}
