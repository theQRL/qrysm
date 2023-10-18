package state_native

import (
	"testing"

	"github.com/theQRL/qrysm/v4/beacon-chain/state"
	testtmpl "github.com/theQRL/qrysm/v4/beacon-chain/state/testing"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
)

func TestBeaconState_SlotDataRace_Phase0(t *testing.T) {
	testtmpl.VerifyBeaconStateSlotDataRace(t, func() (state.BeaconState, error) {
		return InitializeFromProtoPhase0(&zondpb.BeaconState{Slot: 1})
	})
}

func TestBeaconState_SlotDataRace_Altair(t *testing.T) {
	testtmpl.VerifyBeaconStateSlotDataRace(t, func() (state.BeaconState, error) {
		return InitializeFromProtoAltair(&zondpb.BeaconStateAltair{Slot: 1})
	})
}

func TestBeaconState_SlotDataRace_Bellatrix(t *testing.T) {
	testtmpl.VerifyBeaconStateSlotDataRace(t, func() (state.BeaconState, error) {
		return InitializeFromProtoBellatrix(&zondpb.BeaconStateBellatrix{Slot: 1})
	})
}

func TestBeaconState_SlotDataRace_Capella(t *testing.T) {
	testtmpl.VerifyBeaconStateSlotDataRace(t, func() (state.BeaconState, error) {
		return InitializeFromProtoCapella(&zondpb.BeaconStateCapella{Slot: 1})
	})
}

func TestBeaconState_MatchCurrentJustifiedCheckpt_Phase0(t *testing.T) {
	testtmpl.VerifyBeaconStateMatchCurrentJustifiedCheckptNative(
		t,
		func(cp *zondpb.Checkpoint) (state.BeaconState, error) {
			return InitializeFromProtoPhase0(&zondpb.BeaconState{CurrentJustifiedCheckpoint: cp})
		},
	)
}

func TestBeaconState_MatchCurrentJustifiedCheckpt_Altair(t *testing.T) {
	testtmpl.VerifyBeaconStateMatchCurrentJustifiedCheckptNative(
		t,
		func(cp *zondpb.Checkpoint) (state.BeaconState, error) {
			return InitializeFromProtoAltair(&zondpb.BeaconStateAltair{CurrentJustifiedCheckpoint: cp})
		},
	)
}

func TestBeaconState_MatchCurrentJustifiedCheckpt_Bellatrix(t *testing.T) {
	testtmpl.VerifyBeaconStateMatchCurrentJustifiedCheckptNative(
		t,
		func(cp *zondpb.Checkpoint) (state.BeaconState, error) {
			return InitializeFromProtoBellatrix(&zondpb.BeaconStateBellatrix{CurrentJustifiedCheckpoint: cp})
		},
	)
}

func TestBeaconState_MatchCurrentJustifiedCheckpt_Capella(t *testing.T) {
	testtmpl.VerifyBeaconStateMatchCurrentJustifiedCheckptNative(
		t,
		func(cp *zondpb.Checkpoint) (state.BeaconState, error) {
			return InitializeFromProtoCapella(&zondpb.BeaconStateCapella{CurrentJustifiedCheckpoint: cp})
		},
	)
}

func TestBeaconState_MatchPreviousJustifiedCheckpt_Phase0(t *testing.T) {
	testtmpl.VerifyBeaconStateMatchPreviousJustifiedCheckptNative(
		t,
		func(cp *zondpb.Checkpoint) (state.BeaconState, error) {
			return InitializeFromProtoPhase0(&zondpb.BeaconState{PreviousJustifiedCheckpoint: cp})
		},
	)
}

func TestBeaconState_MatchPreviousJustifiedCheckpt_Altair(t *testing.T) {
	testtmpl.VerifyBeaconStateMatchPreviousJustifiedCheckptNative(
		t,
		func(cp *zondpb.Checkpoint) (state.BeaconState, error) {
			return InitializeFromProtoAltair(&zondpb.BeaconStateAltair{PreviousJustifiedCheckpoint: cp})
		},
	)
}

func TestBeaconState_MatchPreviousJustifiedCheckpt_Bellatrix(t *testing.T) {
	testtmpl.VerifyBeaconStateMatchPreviousJustifiedCheckptNative(
		t,
		func(cp *zondpb.Checkpoint) (state.BeaconState, error) {
			return InitializeFromProtoBellatrix(&zondpb.BeaconStateBellatrix{PreviousJustifiedCheckpoint: cp})
		},
	)
}

func TestBeaconState_MatchPreviousJustifiedCheckpt_Capella(t *testing.T) {
	testtmpl.VerifyBeaconStateMatchPreviousJustifiedCheckptNative(
		t,
		func(cp *zondpb.Checkpoint) (state.BeaconState, error) {
			return InitializeFromProtoCapella(&zondpb.BeaconStateCapella{PreviousJustifiedCheckpoint: cp})
		},
	)
}

func TestBeaconState_ValidatorByPubkey_Phase0(t *testing.T) {
	testtmpl.VerifyBeaconStateValidatorByPubkey(t, func() (state.BeaconState, error) {
		return InitializeFromProtoPhase0(&zondpb.BeaconState{})
	})
}

func TestBeaconState_ValidatorByPubkey_Altair(t *testing.T) {
	testtmpl.VerifyBeaconStateValidatorByPubkey(t, func() (state.BeaconState, error) {
		return InitializeFromProtoAltair(&zondpb.BeaconStateAltair{})
	})
}

func TestBeaconState_ValidatorByPubkey_Bellatrix(t *testing.T) {
	testtmpl.VerifyBeaconStateValidatorByPubkey(t, func() (state.BeaconState, error) {
		return InitializeFromProtoBellatrix(&zondpb.BeaconStateBellatrix{})
	})
}

func TestBeaconState_ValidatorByPubkey_Capella(t *testing.T) {
	testtmpl.VerifyBeaconStateValidatorByPubkey(t, func() (state.BeaconState, error) {
		return InitializeFromProtoCapella(&zondpb.BeaconStateCapella{})
	})
}
