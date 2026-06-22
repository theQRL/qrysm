package state_native

import (
	"testing"

	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/qrysm/beacon-chain/state"
	testtmpl "github.com/theQRL/qrysm/beacon-chain/state/testing"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

func TestBeaconState_PreviousJustifiedCheckpointNil_Zond(t *testing.T) {
	testtmpl.VerifyBeaconStatePreviousJustifiedCheckpointNil(
		t,
		func() (state.BeaconState, error) {
			return InitializeFromProtoUnsafeZond(&qrysmpb.BeaconStateZond{})
		})
}

func TestBeaconState_PreviousJustifiedCheckpoint_Zond(t *testing.T) {
	testtmpl.VerifyBeaconStatePreviousJustifiedCheckpoint(
		t,
		func(cp *qrysmpb.Checkpoint) (state.BeaconState, error) {
			return InitializeFromProtoUnsafeZond(&qrysmpb.BeaconStateZond{PreviousJustifiedCheckpoint: cp})
		})
}

func TestBeaconState_CurrentJustifiedCheckpointNil_Zond(t *testing.T) {
	testtmpl.VerifyBeaconStateCurrentJustifiedCheckpointNil(
		t,
		func() (state.BeaconState, error) {
			return InitializeFromProtoUnsafeZond(&qrysmpb.BeaconStateZond{})
		})
}

func TestBeaconState_CurrentJustifiedCheckpoint_Zond(t *testing.T) {
	testtmpl.VerifyBeaconStateCurrentJustifiedCheckpoint(
		t,
		func(cp *qrysmpb.Checkpoint) (state.BeaconState, error) {
			return InitializeFromProtoUnsafeZond(&qrysmpb.BeaconStateZond{CurrentJustifiedCheckpoint: cp})
		})
}

func TestBeaconState_FinalizedCheckpointNil_Zond(t *testing.T) {
	testtmpl.VerifyBeaconStateFinalizedCheckpointNil(
		t,
		func() (state.BeaconState, error) {
			return InitializeFromProtoUnsafeZond(&qrysmpb.BeaconStateZond{})
		})
}

func TestBeaconState_FinalizedCheckpoint_Zond(t *testing.T) {
	testtmpl.VerifyBeaconStateFinalizedCheckpoint(
		t,
		func(cp *qrysmpb.Checkpoint) (state.BeaconState, error) {
			return InitializeFromProtoUnsafeZond(&qrysmpb.BeaconStateZond{FinalizedCheckpoint: cp})
		})
}

func TestBeaconState_JustificationBitsNil_Zond(t *testing.T) {
	testtmpl.VerifyBeaconStateJustificationBitsNil(
		t,
		func() (state.BeaconState, error) {
			return InitializeFromProtoUnsafeZond(&qrysmpb.BeaconStateZond{})
		})
}

func TestBeaconState_JustificationBits_Zond(t *testing.T) {
	testtmpl.VerifyBeaconStateJustificationBits(
		t,
		func(bits bitfield.Bitvector4) (state.BeaconState, error) {
			return InitializeFromProtoUnsafeZond(&qrysmpb.BeaconStateZond{JustificationBits: bits})
		})
}
