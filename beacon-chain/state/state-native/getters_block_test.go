package state_native

import (
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/state"
	testtmpl "github.com/theQRL/qrysm/beacon-chain/state/testing"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

func TestBeaconState_LatestBlockHeader_Zond(t *testing.T) {
	testtmpl.VerifyBeaconStateLatestBlockHeader(
		t,
		func() (state.BeaconState, error) {
			return InitializeFromProtoZond(&qrysmpb.BeaconStateZond{})
		},
		func(BH *qrysmpb.BeaconBlockHeader) (state.BeaconState, error) {
			return InitializeFromProtoZond(&qrysmpb.BeaconStateZond{LatestBlockHeader: BH})
		},
	)
}

func TestBeaconState_BlockRoots_Zond(t *testing.T) {
	testtmpl.VerifyBeaconStateBlockRootsNative(
		t,
		func() (state.BeaconState, error) {
			return InitializeFromProtoZond(&qrysmpb.BeaconStateZond{})
		},
		func(BR [][]byte) (state.BeaconState, error) {
			return InitializeFromProtoZond(&qrysmpb.BeaconStateZond{BlockRoots: BR})
		},
	)
}

func TestBeaconState_BlockRootAtIndex_Zond(t *testing.T) {
	testtmpl.VerifyBeaconStateBlockRootAtIndexNative(
		t,
		func() (state.BeaconState, error) {
			return InitializeFromProtoZond(&qrysmpb.BeaconStateZond{})
		},
		func(BR [][]byte) (state.BeaconState, error) {
			return InitializeFromProtoZond(&qrysmpb.BeaconStateZond{BlockRoots: BR})
		},
	)
}
