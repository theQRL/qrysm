package state_native

import (
	"testing"

	"github.com/theQRL/qrysm/v4/beacon-chain/state"
	testtmpl "github.com/theQRL/qrysm/v4/beacon-chain/state/testing"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
)

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
