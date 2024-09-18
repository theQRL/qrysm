package synccommittee

import (
	"testing"

	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
)

func TestSyncCommitteeContributionCache_Nil(t *testing.T) {
	store := NewStore()
	require.Equal(t, errNilContribution, store.SaveSyncCommitteeContribution(nil))
}

func TestSyncCommitteeContributionCache_RoundTrip(t *testing.T) {
	store := NewStore()

	conts := []*zondpb.SyncCommitteeContribution{
		{Slot: 1, SubcommitteeIndex: 0, Signatures: [][]byte{[]byte{'a'}}},
		{Slot: 1, SubcommitteeIndex: 1, Signatures: [][]byte{[]byte{'b'}}},
		{Slot: 2, SubcommitteeIndex: 0, Signatures: [][]byte{[]byte{'c'}}},
		{Slot: 2, SubcommitteeIndex: 1, Signatures: [][]byte{[]byte{'d'}}},
		{Slot: 3, SubcommitteeIndex: 0, Signatures: [][]byte{[]byte{'e'}}},
		{Slot: 3, SubcommitteeIndex: 1, Signatures: [][]byte{[]byte{'f'}}},
		{Slot: 4, SubcommitteeIndex: 0, Signatures: [][]byte{[]byte{'g'}}},
		{Slot: 4, SubcommitteeIndex: 1, Signatures: [][]byte{[]byte{'h'}}},
		{Slot: 5, SubcommitteeIndex: 0, Signatures: [][]byte{[]byte{'i'}}},
		{Slot: 5, SubcommitteeIndex: 1, Signatures: [][]byte{[]byte{'j'}}},
		{Slot: 6, SubcommitteeIndex: 0, Signatures: [][]byte{[]byte{'k'}}},
		{Slot: 6, SubcommitteeIndex: 1, Signatures: [][]byte{[]byte{'l'}}},
	}

	for _, sig := range conts {
		require.NoError(t, store.SaveSyncCommitteeContribution(sig))
	}

	conts, err := store.SyncCommitteeContributions(1)
	require.NoError(t, err)
	require.DeepSSZEqual(t, []*zondpb.SyncCommitteeContribution{}, conts)

	conts, err = store.SyncCommitteeContributions(2)
	require.NoError(t, err)
	require.DeepSSZEqual(t, []*zondpb.SyncCommitteeContribution{}, conts)

	conts, err = store.SyncCommitteeContributions(3)
	require.NoError(t, err)
	require.DeepSSZEqual(t, []*zondpb.SyncCommitteeContribution{
		{Slot: 3, SubcommitteeIndex: 0, Signatures: [][]byte{[]byte{'e'}}},
		{Slot: 3, SubcommitteeIndex: 1, Signatures: [][]byte{[]byte{'f'}}},
	}, conts)

	conts, err = store.SyncCommitteeContributions(4)
	require.NoError(t, err)
	require.DeepSSZEqual(t, []*zondpb.SyncCommitteeContribution{
		{Slot: 4, SubcommitteeIndex: 0, Signatures: [][]byte{[]byte{'g'}}},
		{Slot: 4, SubcommitteeIndex: 1, Signatures: [][]byte{[]byte{'h'}}},
	}, conts)

	conts, err = store.SyncCommitteeContributions(5)
	require.NoError(t, err)
	require.DeepSSZEqual(t, []*zondpb.SyncCommitteeContribution{
		{Slot: 5, SubcommitteeIndex: 0, Signatures: [][]byte{[]byte{'i'}}},
		{Slot: 5, SubcommitteeIndex: 1, Signatures: [][]byte{[]byte{'j'}}},
	}, conts)

	conts, err = store.SyncCommitteeContributions(6)
	require.NoError(t, err)
	require.DeepSSZEqual(t, []*zondpb.SyncCommitteeContribution{
		{Slot: 6, SubcommitteeIndex: 0, Signatures: [][]byte{[]byte{'k'}}},
		{Slot: 6, SubcommitteeIndex: 1, Signatures: [][]byte{[]byte{'l'}}},
	}, conts)

	// All the contributions should persist after get.
	conts, err = store.SyncCommitteeContributions(1)
	require.NoError(t, err)
	require.DeepSSZEqual(t, []*zondpb.SyncCommitteeContribution{}, conts)
	conts, err = store.SyncCommitteeContributions(2)
	require.NoError(t, err)
	require.DeepSSZEqual(t, []*zondpb.SyncCommitteeContribution{}, conts)

	conts, err = store.SyncCommitteeContributions(3)
	require.NoError(t, err)
	require.DeepSSZEqual(t, []*zondpb.SyncCommitteeContribution{
		{Slot: 3, SubcommitteeIndex: 0, Signatures: [][]byte{[]byte{'e'}}},
		{Slot: 3, SubcommitteeIndex: 1, Signatures: [][]byte{[]byte{'f'}}},
	}, conts)

	conts, err = store.SyncCommitteeContributions(4)
	require.NoError(t, err)
	require.DeepSSZEqual(t, []*zondpb.SyncCommitteeContribution{
		{Slot: 4, SubcommitteeIndex: 0, Signatures: [][]byte{[]byte{'g'}}},
		{Slot: 4, SubcommitteeIndex: 1, Signatures: [][]byte{[]byte{'h'}}},
	}, conts)

	conts, err = store.SyncCommitteeContributions(5)
	require.NoError(t, err)
	require.DeepSSZEqual(t, []*zondpb.SyncCommitteeContribution{
		{Slot: 5, SubcommitteeIndex: 0, Signatures: [][]byte{[]byte{'i'}}},
		{Slot: 5, SubcommitteeIndex: 1, Signatures: [][]byte{[]byte{'j'}}},
	}, conts)

	conts, err = store.SyncCommitteeContributions(6)
	require.NoError(t, err)
	require.DeepSSZEqual(t, []*zondpb.SyncCommitteeContribution{
		{Slot: 6, SubcommitteeIndex: 0, Signatures: [][]byte{[]byte{'k'}}},
		{Slot: 6, SubcommitteeIndex: 1, Signatures: [][]byte{[]byte{'l'}}},
	}, conts)
}
