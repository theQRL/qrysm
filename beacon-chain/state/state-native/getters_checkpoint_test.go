package state_native

import (
	"testing"

	"github.com/prysmaticlabs/go-bitfield"
	"github.com/theQRL/qrysm/v4/beacon-chain/state"
	testtmpl "github.com/theQRL/qrysm/v4/beacon-chain/state/testing"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
)

func TestBeaconState_PreviousJustifiedCheckpointNil_Phase0(t *testing.T) {
	testtmpl.VerifyBeaconStatePreviousJustifiedCheckpointNil(
		t,
		func() (state.BeaconState, error) {
			return InitializeFromProtoUnsafePhase0(&zondpb.BeaconState{})
		})
}

func TestBeaconState_PreviousJustifiedCheckpointNil_Altair(t *testing.T) {
	testtmpl.VerifyBeaconStatePreviousJustifiedCheckpointNil(
		t,
		func() (state.BeaconState, error) {
			return InitializeFromProtoUnsafeAltair(&zondpb.BeaconStateAltair{})
		})
}

func TestBeaconState_PreviousJustifiedCheckpointNil_Bellatrix(t *testing.T) {
	testtmpl.VerifyBeaconStatePreviousJustifiedCheckpointNil(
		t,
		func() (state.BeaconState, error) {
			return InitializeFromProtoUnsafeBellatrix(&zondpb.BeaconStateBellatrix{})
		})
}

func TestBeaconState_PreviousJustifiedCheckpointNil_Capella(t *testing.T) {
	testtmpl.VerifyBeaconStatePreviousJustifiedCheckpointNil(
		t,
		func() (state.BeaconState, error) {
			return InitializeFromProtoUnsafeCapella(&zondpb.BeaconStateCapella{})
		})
}

func TestBeaconState_PreviousJustifiedCheckpointNil_Deneb(t *testing.T) {
	testtmpl.VerifyBeaconStatePreviousJustifiedCheckpointNil(
		t,
		func() (state.BeaconState, error) {
			return InitializeFromProtoUnsafeDeneb(&zondpb.BeaconStateDeneb{})
		})
}

func TestBeaconState_PreviousJustifiedCheckpoint_Phase0(t *testing.T) {
	testtmpl.VerifyBeaconStatePreviousJustifiedCheckpoint(
		t,
		func(cp *zondpb.Checkpoint) (state.BeaconState, error) {
			return InitializeFromProtoUnsafePhase0(&zondpb.BeaconState{PreviousJustifiedCheckpoint: cp})
		})
}

func TestBeaconState_PreviousJustifiedCheckpoint_Altair(t *testing.T) {
	testtmpl.VerifyBeaconStatePreviousJustifiedCheckpoint(
		t,
		func(cp *zondpb.Checkpoint) (state.BeaconState, error) {
			return InitializeFromProtoUnsafeAltair(&zondpb.BeaconStateAltair{PreviousJustifiedCheckpoint: cp})
		})
}

func TestBeaconState_PreviousJustifiedCheckpoint_Bellatrix(t *testing.T) {
	testtmpl.VerifyBeaconStatePreviousJustifiedCheckpoint(
		t,
		func(cp *zondpb.Checkpoint) (state.BeaconState, error) {
			return InitializeFromProtoUnsafeBellatrix(&zondpb.BeaconStateBellatrix{PreviousJustifiedCheckpoint: cp})
		})
}

func TestBeaconState_PreviousJustifiedCheckpoint_Capella(t *testing.T) {
	testtmpl.VerifyBeaconStatePreviousJustifiedCheckpoint(
		t,
		func(cp *zondpb.Checkpoint) (state.BeaconState, error) {
			return InitializeFromProtoUnsafeCapella(&zondpb.BeaconStateCapella{PreviousJustifiedCheckpoint: cp})
		})
}

func TestBeaconState_PreviousJustifiedCheckpoint_Deneb(t *testing.T) {
	testtmpl.VerifyBeaconStatePreviousJustifiedCheckpoint(
		t,
		func(cp *zondpb.Checkpoint) (state.BeaconState, error) {
			return InitializeFromProtoUnsafeDeneb(&zondpb.BeaconStateDeneb{PreviousJustifiedCheckpoint: cp})
		})
}

func TestBeaconState_CurrentJustifiedCheckpointNil_Phase0(t *testing.T) {
	testtmpl.VerifyBeaconStateCurrentJustifiedCheckpointNil(
		t,
		func() (state.BeaconState, error) {
			return InitializeFromProtoUnsafePhase0(&zondpb.BeaconState{})
		})
}

func TestBeaconState_CurrentJustifiedCheckpointNil_Altair(t *testing.T) {
	testtmpl.VerifyBeaconStateCurrentJustifiedCheckpointNil(
		t,
		func() (state.BeaconState, error) {
			return InitializeFromProtoUnsafeAltair(&zondpb.BeaconStateAltair{})
		})
}

func TestBeaconState_CurrentJustifiedCheckpointNil_Bellatrix(t *testing.T) {
	testtmpl.VerifyBeaconStateCurrentJustifiedCheckpointNil(
		t,
		func() (state.BeaconState, error) {
			return InitializeFromProtoUnsafeBellatrix(&zondpb.BeaconStateBellatrix{})
		})
}

func TestBeaconState_CurrentJustifiedCheckpointNil_Capella(t *testing.T) {
	testtmpl.VerifyBeaconStateCurrentJustifiedCheckpointNil(
		t,
		func() (state.BeaconState, error) {
			return InitializeFromProtoUnsafeCapella(&zondpb.BeaconStateCapella{})
		})
}

func TestBeaconState_CurrentJustifiedCheckpointNil_Deneb(t *testing.T) {
	testtmpl.VerifyBeaconStateCurrentJustifiedCheckpointNil(
		t,
		func() (state.BeaconState, error) {
			return InitializeFromProtoUnsafeDeneb(&zondpb.BeaconStateDeneb{})
		})
}

func TestBeaconState_CurrentJustifiedCheckpoint_Phase0(t *testing.T) {
	testtmpl.VerifyBeaconStateCurrentJustifiedCheckpoint(
		t,
		func(cp *zondpb.Checkpoint) (state.BeaconState, error) {
			return InitializeFromProtoUnsafePhase0(&zondpb.BeaconState{CurrentJustifiedCheckpoint: cp})
		})
}

func TestBeaconState_CurrentJustifiedCheckpoint_Altair(t *testing.T) {
	testtmpl.VerifyBeaconStateCurrentJustifiedCheckpoint(
		t,
		func(cp *zondpb.Checkpoint) (state.BeaconState, error) {
			return InitializeFromProtoUnsafeAltair(&zondpb.BeaconStateAltair{CurrentJustifiedCheckpoint: cp})
		})
}

func TestBeaconState_CurrentJustifiedCheckpoint_Bellatrix(t *testing.T) {
	testtmpl.VerifyBeaconStateCurrentJustifiedCheckpoint(
		t,
		func(cp *zondpb.Checkpoint) (state.BeaconState, error) {
			return InitializeFromProtoUnsafeBellatrix(&zondpb.BeaconStateBellatrix{CurrentJustifiedCheckpoint: cp})
		})
}

func TestBeaconState_CurrentJustifiedCheckpoint_Capella(t *testing.T) {
	testtmpl.VerifyBeaconStateCurrentJustifiedCheckpoint(
		t,
		func(cp *zondpb.Checkpoint) (state.BeaconState, error) {
			return InitializeFromProtoUnsafeCapella(&zondpb.BeaconStateCapella{CurrentJustifiedCheckpoint: cp})
		})
}

func TestBeaconState_CurrentJustifiedCheckpoint_Deneb(t *testing.T) {
	testtmpl.VerifyBeaconStateCurrentJustifiedCheckpoint(
		t,
		func(cp *zondpb.Checkpoint) (state.BeaconState, error) {
			return InitializeFromProtoUnsafeDeneb(&zondpb.BeaconStateDeneb{CurrentJustifiedCheckpoint: cp})
		})
}

func TestBeaconState_FinalizedCheckpointNil_Phase0(t *testing.T) {
	testtmpl.VerifyBeaconStateFinalizedCheckpointNil(
		t,
		func() (state.BeaconState, error) {
			return InitializeFromProtoUnsafePhase0(&zondpb.BeaconState{})
		})
}

func TestBeaconState_FinalizedCheckpointNil_Altair(t *testing.T) {
	testtmpl.VerifyBeaconStateFinalizedCheckpointNil(
		t,
		func() (state.BeaconState, error) {
			return InitializeFromProtoUnsafeAltair(&zondpb.BeaconStateAltair{})
		})
}

func TestBeaconState_FinalizedCheckpointNil_Bellatrix(t *testing.T) {
	testtmpl.VerifyBeaconStateFinalizedCheckpointNil(
		t,
		func() (state.BeaconState, error) {
			return InitializeFromProtoUnsafeBellatrix(&zondpb.BeaconStateBellatrix{})
		})
}

func TestBeaconState_FinalizedCheckpointNil_Capella(t *testing.T) {
	testtmpl.VerifyBeaconStateFinalizedCheckpointNil(
		t,
		func() (state.BeaconState, error) {
			return InitializeFromProtoUnsafeCapella(&zondpb.BeaconStateCapella{})
		})
}

func TestBeaconState_FinalizedCheckpointNil_Deneb(t *testing.T) {
	testtmpl.VerifyBeaconStateFinalizedCheckpointNil(
		t,
		func() (state.BeaconState, error) {
			return InitializeFromProtoUnsafeDeneb(&zondpb.BeaconStateDeneb{})
		})
}

func TestBeaconState_FinalizedCheckpoint_Phase0(t *testing.T) {
	testtmpl.VerifyBeaconStateFinalizedCheckpoint(
		t,
		func(cp *zondpb.Checkpoint) (state.BeaconState, error) {
			return InitializeFromProtoUnsafePhase0(&zondpb.BeaconState{FinalizedCheckpoint: cp})
		})
}

func TestBeaconState_FinalizedCheckpoint_Altair(t *testing.T) {
	testtmpl.VerifyBeaconStateFinalizedCheckpoint(
		t,
		func(cp *zondpb.Checkpoint) (state.BeaconState, error) {
			return InitializeFromProtoUnsafeAltair(&zondpb.BeaconStateAltair{FinalizedCheckpoint: cp})
		})
}

func TestBeaconState_FinalizedCheckpoint_Bellatrix(t *testing.T) {
	testtmpl.VerifyBeaconStateFinalizedCheckpoint(
		t,
		func(cp *zondpb.Checkpoint) (state.BeaconState, error) {
			return InitializeFromProtoUnsafeBellatrix(&zondpb.BeaconStateBellatrix{FinalizedCheckpoint: cp})
		})
}

func TestBeaconState_FinalizedCheckpoint_Capella(t *testing.T) {
	testtmpl.VerifyBeaconStateFinalizedCheckpoint(
		t,
		func(cp *zondpb.Checkpoint) (state.BeaconState, error) {
			return InitializeFromProtoUnsafeCapella(&zondpb.BeaconStateCapella{FinalizedCheckpoint: cp})
		})
}

func TestBeaconState_JustificationBitsNil_Phase0(t *testing.T) {
	testtmpl.VerifyBeaconStateJustificationBitsNil(
		t,
		func() (state.BeaconState, error) {
			return InitializeFromProtoUnsafePhase0(&zondpb.BeaconState{})
		})
}

func TestBeaconState_JustificationBitsNil_Altair(t *testing.T) {
	testtmpl.VerifyBeaconStateJustificationBitsNil(
		t,
		func() (state.BeaconState, error) {
			return InitializeFromProtoUnsafeAltair(&zondpb.BeaconStateAltair{})
		})
}

func TestBeaconState_JustificationBitsNil_Bellatrix(t *testing.T) {
	testtmpl.VerifyBeaconStateJustificationBitsNil(
		t,
		func() (state.BeaconState, error) {
			return InitializeFromProtoUnsafeBellatrix(&zondpb.BeaconStateBellatrix{})
		})
}

func TestBeaconState_JustificationBitsNil_Capella(t *testing.T) {
	testtmpl.VerifyBeaconStateJustificationBitsNil(
		t,
		func() (state.BeaconState, error) {
			return InitializeFromProtoUnsafeCapella(&zondpb.BeaconStateCapella{})
		})
}

func TestBeaconState_JustificationBitsNil_Deneb(t *testing.T) {
	testtmpl.VerifyBeaconStateJustificationBitsNil(
		t,
		func() (state.BeaconState, error) {
			return InitializeFromProtoUnsafeDeneb(&zondpb.BeaconStateDeneb{})
		})
}

func TestBeaconState_FinalizedCheckpoint_Deneb(t *testing.T) {
	testtmpl.VerifyBeaconStateFinalizedCheckpoint(
		t,
		func(cp *zondpb.Checkpoint) (state.BeaconState, error) {
			return InitializeFromProtoUnsafeDeneb(&zondpb.BeaconStateDeneb{FinalizedCheckpoint: cp})
		})
}

func TestBeaconState_JustificationBits_Phase0(t *testing.T) {
	testtmpl.VerifyBeaconStateJustificationBits(
		t,
		func(bits bitfield.Bitvector4) (state.BeaconState, error) {
			return InitializeFromProtoUnsafePhase0(&zondpb.BeaconState{JustificationBits: bits})
		})
}

func TestBeaconState_JustificationBits_Altair(t *testing.T) {
	testtmpl.VerifyBeaconStateJustificationBits(
		t,
		func(bits bitfield.Bitvector4) (state.BeaconState, error) {
			return InitializeFromProtoUnsafeAltair(&zondpb.BeaconStateAltair{JustificationBits: bits})
		})
}

func TestBeaconState_JustificationBits_Bellatrix(t *testing.T) {
	testtmpl.VerifyBeaconStateJustificationBits(
		t,
		func(bits bitfield.Bitvector4) (state.BeaconState, error) {
			return InitializeFromProtoUnsafeBellatrix(&zondpb.BeaconStateBellatrix{JustificationBits: bits})
		})
}

func TestBeaconState_JustificationBits_Capella(t *testing.T) {
	testtmpl.VerifyBeaconStateJustificationBits(
		t,
		func(bits bitfield.Bitvector4) (state.BeaconState, error) {
			return InitializeFromProtoUnsafeCapella(&zondpb.BeaconStateCapella{JustificationBits: bits})
		})
}

func TestBeaconState_JustificationBits_Deneb(t *testing.T) {
	testtmpl.VerifyBeaconStateJustificationBits(
		t,
		func(bits bitfield.Bitvector4) (state.BeaconState, error) {
			return InitializeFromProtoUnsafeDeneb(&zondpb.BeaconStateDeneb{JustificationBits: bits})
		})
}
