package testing

import (
	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/qrysm/beacon-chain/state"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"

	"testing"
)

func VerifyBeaconStateJustificationBitsNil(t *testing.T, factory getState) {
	s, err := factory()
	require.NoError(t, err)
	require.DeepEqual(t, bitfield.Bitvector4{}.Bytes(), s.JustificationBits().Bytes())
}

type getStateWithJustificationBits = func(bitfield.Bitvector4) (state.BeaconState, error)

func VerifyBeaconStateJustificationBits(t *testing.T, factory getStateWithJustificationBits) {
	s, err := factory(bitfield.Bitvector4{1, 2, 3, 4})
	require.NoError(t, err)
	require.DeepEqual(t, bitfield.Bitvector4{1, 2, 3, 4}.Bytes(), s.JustificationBits().Bytes())
}

func VerifyBeaconStatePreviousJustifiedCheckpointNil(t *testing.T, factory getState) {
	s, err := factory()

	require.NoError(t, err)

	checkpoint := s.PreviousJustifiedCheckpoint()
	require.Equal(t, (*qrysmpb.Checkpoint)(nil), checkpoint)
}

type getStateWithCheckpoint = func(checkpoint *qrysmpb.Checkpoint) (state.BeaconState, error)

func VerifyBeaconStatePreviousJustifiedCheckpoint(t *testing.T, factory getStateWithCheckpoint) {
	orgCheckpoint := &qrysmpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)}
	orgCheckpoint.Root[1] = 1
	orgCheckpoint.Root[2] = 2
	orgCheckpoint.Root[3] = 3
	s, err := factory(orgCheckpoint)

	require.NoError(t, err)

	checkpoint := s.PreviousJustifiedCheckpoint()
	require.DeepEqual(t, orgCheckpoint.Root, checkpoint.Root)
}

func VerifyBeaconStateCurrentJustifiedCheckpointNil(t *testing.T, factory getState) {
	s, err := factory()

	require.NoError(t, err)

	checkpoint := s.CurrentJustifiedCheckpoint()
	require.Equal(t, (*qrysmpb.Checkpoint)(nil), checkpoint)
}

func VerifyBeaconStateCurrentJustifiedCheckpoint(t *testing.T, factory getStateWithCheckpoint) {
	orgCheckpoint := &qrysmpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)}
	orgCheckpoint.Root[1] = 1
	orgCheckpoint.Root[2] = 2
	orgCheckpoint.Root[3] = 3
	s, err := factory(orgCheckpoint)

	require.NoError(t, err)

	checkpoint := s.CurrentJustifiedCheckpoint()
	require.DeepEqual(t, orgCheckpoint.Root, checkpoint.Root)
}

func VerifyBeaconStateFinalizedCheckpointNil(t *testing.T, factory getState) {
	s, err := factory()

	require.NoError(t, err)

	checkpoint := s.FinalizedCheckpoint()
	require.Equal(t, (*qrysmpb.Checkpoint)(nil), checkpoint)
	epoch := s.FinalizedCheckpointEpoch()
	require.Equal(t, primitives.Epoch(0), epoch)
}

func VerifyBeaconStateFinalizedCheckpoint(t *testing.T, factory getStateWithCheckpoint) {
	orgCheckpoint := &qrysmpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)}
	orgCheckpoint.Root[1] = 1
	orgCheckpoint.Root[2] = 2
	orgCheckpoint.Root[3] = 3
	orgCheckpoint.Epoch = 123
	s, err := factory(orgCheckpoint)

	require.NoError(t, err)

	checkpoint := s.FinalizedCheckpoint()
	require.DeepEqual(t, orgCheckpoint.Root, checkpoint.Root)
	epoch := s.FinalizedCheckpointEpoch()
	require.Equal(t, orgCheckpoint.Epoch, epoch)
}
