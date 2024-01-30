package testing

import (
	"sync"
	"testing"

	dilithium2 "github.com/theQRL/go-qrllib/dilithium"
	"github.com/theQRL/qrysm/v4/beacon-chain/state"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
)

func VerifyBeaconStateSlotDataRace(t *testing.T, factory getState) {
	headState, err := factory()
	require.NoError(t, err)

	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		require.NoError(t, headState.SetSlot(0))
		wg.Done()
	}()
	go func() {
		headState.Slot()
		wg.Done()
	}()

	wg.Wait()
}

type getStateWithCurrentJustifiedCheckpoint func(*zondpb.Checkpoint) (state.BeaconState, error)

func VerifyBeaconStateMatchCurrentJustifiedCheckptNative(t *testing.T, factory getStateWithCurrentJustifiedCheckpoint) {
	c1 := &zondpb.Checkpoint{Epoch: 1}
	c2 := &zondpb.Checkpoint{Epoch: 2}
	beaconState, err := factory(c1)
	require.NoError(t, err)
	require.Equal(t, true, beaconState.MatchCurrentJustifiedCheckpoint(c1))
	require.Equal(t, false, beaconState.MatchCurrentJustifiedCheckpoint(c2))
	require.Equal(t, false, beaconState.MatchPreviousJustifiedCheckpoint(c1))
	require.Equal(t, false, beaconState.MatchPreviousJustifiedCheckpoint(c2))
}

func VerifyBeaconStateMatchPreviousJustifiedCheckptNative(t *testing.T, factory getStateWithCurrentJustifiedCheckpoint) {
	c1 := &zondpb.Checkpoint{Epoch: 1}
	c2 := &zondpb.Checkpoint{Epoch: 2}
	beaconState, err := factory(c1)
	require.NoError(t, err)
	require.Equal(t, false, beaconState.MatchCurrentJustifiedCheckpoint(c1))
	require.Equal(t, false, beaconState.MatchCurrentJustifiedCheckpoint(c2))
	require.Equal(t, true, beaconState.MatchPreviousJustifiedCheckpoint(c1))
	require.Equal(t, false, beaconState.MatchPreviousJustifiedCheckpoint(c2))
}

func VerifyBeaconStateValidatorByPubkey(t *testing.T, factory getState) {
	keyCreator := func(input []byte) [dilithium2.CryptoPublicKeyBytes]byte {
		var nKey [dilithium2.CryptoPublicKeyBytes]byte
		copy(nKey[:1], input)
		return nKey
	}

	tests := []struct {
		name            string
		modifyFunc      func(b state.BeaconState, k [dilithium2.CryptoPublicKeyBytes]byte)
		exists          bool
		expectedIdx     primitives.ValidatorIndex
		largestIdxInSet primitives.ValidatorIndex
	}{
		{
			name: "retrieve validator",
			modifyFunc: func(b state.BeaconState, key [dilithium2.CryptoPublicKeyBytes]byte) {
				assert.NoError(t, b.AppendValidator(&zondpb.Validator{PublicKey: key[:]}))
			},
			exists:      true,
			expectedIdx: 0,
		},
		{
			name: "retrieve validator with multiple validators from the start",
			modifyFunc: func(b state.BeaconState, key [dilithium2.CryptoPublicKeyBytes]byte) {
				key1 := keyCreator([]byte{'C'})
				key2 := keyCreator([]byte{'D'})
				assert.NoError(t, b.AppendValidator(&zondpb.Validator{PublicKey: key[:]}))
				assert.NoError(t, b.AppendValidator(&zondpb.Validator{PublicKey: key1[:]}))
				assert.NoError(t, b.AppendValidator(&zondpb.Validator{PublicKey: key2[:]}))
			},
			exists:      true,
			expectedIdx: 0,
		},
		{
			name: "retrieve validator with multiple validators",
			modifyFunc: func(b state.BeaconState, key [dilithium2.CryptoPublicKeyBytes]byte) {
				key1 := keyCreator([]byte{'C'})
				key2 := keyCreator([]byte{'D'})
				assert.NoError(t, b.AppendValidator(&zondpb.Validator{PublicKey: key1[:]}))
				assert.NoError(t, b.AppendValidator(&zondpb.Validator{PublicKey: key2[:]}))
				assert.NoError(t, b.AppendValidator(&zondpb.Validator{PublicKey: key[:]}))
			},
			exists:      true,
			expectedIdx: 2,
		},
		{
			name: "retrieve validator with multiple validators from the start with shared state",
			modifyFunc: func(b state.BeaconState, key [dilithium2.CryptoPublicKeyBytes]byte) {
				key1 := keyCreator([]byte{'C'})
				key2 := keyCreator([]byte{'D'})
				assert.NoError(t, b.AppendValidator(&zondpb.Validator{PublicKey: key[:]}))
				_ = b.Copy()
				assert.NoError(t, b.AppendValidator(&zondpb.Validator{PublicKey: key1[:]}))
				assert.NoError(t, b.AppendValidator(&zondpb.Validator{PublicKey: key2[:]}))
			},
			exists:      true,
			expectedIdx: 0,
		},
		{
			name: "retrieve validator with multiple validators with shared state",
			modifyFunc: func(b state.BeaconState, key [dilithium2.CryptoPublicKeyBytes]byte) {
				key1 := keyCreator([]byte{'C'})
				key2 := keyCreator([]byte{'D'})
				assert.NoError(t, b.AppendValidator(&zondpb.Validator{PublicKey: key1[:]}))
				assert.NoError(t, b.AppendValidator(&zondpb.Validator{PublicKey: key2[:]}))
				n := b.Copy()
				// Append to another state
				assert.NoError(t, n.AppendValidator(&zondpb.Validator{PublicKey: key[:]}))
			},
			exists:      false,
			expectedIdx: 0,
		},
		{
			name: "retrieve validator with multiple validators with shared state at boundary",
			modifyFunc: func(b state.BeaconState, key [dilithium2.CryptoPublicKeyBytes]byte) {
				key1 := keyCreator([]byte{'C'})
				assert.NoError(t, b.AppendValidator(&zondpb.Validator{PublicKey: key1[:]}))
				n := b.Copy()
				// Append to another state
				assert.NoError(t, n.AppendValidator(&zondpb.Validator{PublicKey: key[:]}))
			},
			exists:      false,
			expectedIdx: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := factory()
			require.NoError(t, err)
			nKey := keyCreator([]byte{'A'})
			tt.modifyFunc(s, nKey)
			idx, ok := s.ValidatorIndexByPubkey(nKey)
			assert.Equal(t, tt.exists, ok)
			assert.Equal(t, tt.expectedIdx, idx)
		})
	}
}
