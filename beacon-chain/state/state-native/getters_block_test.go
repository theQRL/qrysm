package state_native

import (
	"testing"

	"github.com/theQRL/qrysm/v4/beacon-chain/state"
	testtmpl "github.com/theQRL/qrysm/v4/beacon-chain/state/testing"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
)

func TestBeaconState_LatestBlockHeader_Phase0(t *testing.T) {
	testtmpl.VerifyBeaconStateLatestBlockHeader(
		t,
		func() (state.BeaconState, error) {
			return InitializeFromProtoPhase0(&zondpb.BeaconState{})
		},
		func(BH *zondpb.BeaconBlockHeader) (state.BeaconState, error) {
			return InitializeFromProtoPhase0(&zondpb.BeaconState{LatestBlockHeader: BH})
		},
	)
}

func TestBeaconState_LatestBlockHeader_Altair(t *testing.T) {
	testtmpl.VerifyBeaconStateLatestBlockHeader(
		t,
		func() (state.BeaconState, error) {
			return InitializeFromProtoAltair(&zondpb.BeaconStateAltair{})
		},
		func(BH *zondpb.BeaconBlockHeader) (state.BeaconState, error) {
			return InitializeFromProtoAltair(&zondpb.BeaconStateAltair{LatestBlockHeader: BH})
		},
	)
}

func TestBeaconState_LatestBlockHeader_Bellatrix(t *testing.T) {
	testtmpl.VerifyBeaconStateLatestBlockHeader(
		t,
		func() (state.BeaconState, error) {
			return InitializeFromProtoBellatrix(&zondpb.BeaconStateBellatrix{})
		},
		func(BH *zondpb.BeaconBlockHeader) (state.BeaconState, error) {
			return InitializeFromProtoBellatrix(&zondpb.BeaconStateBellatrix{LatestBlockHeader: BH})
		},
	)
}

func TestBeaconState_LatestBlockHeader_Capella(t *testing.T) {
	testtmpl.VerifyBeaconStateLatestBlockHeader(
		t,
		func() (state.BeaconState, error) {
			return InitializeFromProtoCapella(&zondpb.BeaconStateCapella{})
		},
		func(BH *zondpb.BeaconBlockHeader) (state.BeaconState, error) {
			return InitializeFromProtoCapella(&zondpb.BeaconStateCapella{LatestBlockHeader: BH})
		},
	)
}

func TestBeaconState_BlockRoots_Phase0(t *testing.T) {
	testtmpl.VerifyBeaconStateBlockRootsNative(
		t,
		func() (state.BeaconState, error) {
			return InitializeFromProtoPhase0(&zondpb.BeaconState{})
		},
		func(BR [][]byte) (state.BeaconState, error) {
			return InitializeFromProtoPhase0(&zondpb.BeaconState{BlockRoots: BR})
		},
	)
}

func TestBeaconState_BlockRoots_Altair(t *testing.T) {
	testtmpl.VerifyBeaconStateBlockRootsNative(
		t,
		func() (state.BeaconState, error) {
			return InitializeFromProtoAltair(&zondpb.BeaconStateAltair{})
		},
		func(BR [][]byte) (state.BeaconState, error) {
			return InitializeFromProtoAltair(&zondpb.BeaconStateAltair{BlockRoots: BR})
		},
	)
}

func TestBeaconState_BlockRoots_Bellatrix(t *testing.T) {
	testtmpl.VerifyBeaconStateBlockRootsNative(
		t,
		func() (state.BeaconState, error) {
			return InitializeFromProtoBellatrix(&zondpb.BeaconStateBellatrix{})
		},
		func(BR [][]byte) (state.BeaconState, error) {
			return InitializeFromProtoBellatrix(&zondpb.BeaconStateBellatrix{BlockRoots: BR})
		},
	)
}

func TestBeaconState_BlockRoots_Capella(t *testing.T) {
	testtmpl.VerifyBeaconStateBlockRootsNative(
		t,
		func() (state.BeaconState, error) {
			return InitializeFromProtoCapella(&zondpb.BeaconStateCapella{})
		},
		func(BR [][]byte) (state.BeaconState, error) {
			return InitializeFromProtoCapella(&zondpb.BeaconStateCapella{BlockRoots: BR})
		},
	)
}

func TestBeaconState_BlockRootAtIndex_Phase0(t *testing.T) {
	testtmpl.VerifyBeaconStateBlockRootAtIndexNative(
		t,
		func() (state.BeaconState, error) {
			return InitializeFromProtoPhase0(&zondpb.BeaconState{})
		},
		func(BR [][]byte) (state.BeaconState, error) {
			return InitializeFromProtoPhase0(&zondpb.BeaconState{BlockRoots: BR})
		},
	)
}

func TestBeaconState_BlockRootAtIndex_Altair(t *testing.T) {
	testtmpl.VerifyBeaconStateBlockRootAtIndexNative(
		t,
		func() (state.BeaconState, error) {
			return InitializeFromProtoAltair(&zondpb.BeaconStateAltair{})
		},
		func(BR [][]byte) (state.BeaconState, error) {
			return InitializeFromProtoAltair(&zondpb.BeaconStateAltair{BlockRoots: BR})
		},
	)
}

func TestBeaconState_BlockRootAtIndex_Bellatrix(t *testing.T) {
	testtmpl.VerifyBeaconStateBlockRootAtIndexNative(
		t,
		func() (state.BeaconState, error) {
			return InitializeFromProtoBellatrix(&zondpb.BeaconStateBellatrix{})
		},
		func(BR [][]byte) (state.BeaconState, error) {
			return InitializeFromProtoBellatrix(&zondpb.BeaconStateBellatrix{BlockRoots: BR})
		},
	)
}

func TestBeaconState_BlockRootAtIndex_Capella(t *testing.T) {
	testtmpl.VerifyBeaconStateBlockRootAtIndexNative(
		t,
		func() (state.BeaconState, error) {
			return InitializeFromProtoCapella(&zondpb.BeaconStateCapella{})
		},
		func(BR [][]byte) (state.BeaconState, error) {
			return InitializeFromProtoCapella(&zondpb.BeaconStateCapella{BlockRoots: BR})
		},
	)
}
