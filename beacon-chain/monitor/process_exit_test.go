package monitor

import (
	"testing"

	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/theQRL/qrysm/v4/consensus-types/blocks"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/require"
)

func TestProcessExitsFromBlockTrackedIndices(t *testing.T) {
	hook := logTest.NewGlobal()
	s := &Service{
		TrackedValidators: map[primitives.ValidatorIndex]bool{
			1: true,
			2: true,
		},
	}

	exits := []*zondpb.SignedVoluntaryExit{
		{
			Exit: &zondpb.VoluntaryExit{
				ValidatorIndex: 3,
				Epoch:          1,
			},
		},
		{
			Exit: &zondpb.VoluntaryExit{
				ValidatorIndex: 2,
				Epoch:          0,
			},
		},
	}

	block := &zondpb.BeaconBlock{
		Body: &zondpb.BeaconBlockBody{
			VoluntaryExits: exits,
		},
	}

	wb, err := blocks.NewBeaconBlock(block)
	require.NoError(t, err)
	s.processExitsFromBlock(wb)
	require.LogsContain(t, hook, "\"Voluntary exit was included\" Slot=0 ValidatorIndex=2")
}

func TestProcessExitsFromBlockUntrackedIndices(t *testing.T) {
	hook := logTest.NewGlobal()
	s := &Service{
		TrackedValidators: map[primitives.ValidatorIndex]bool{
			1: true,
			2: true,
		},
	}

	exits := []*zondpb.SignedVoluntaryExit{
		{
			Exit: &zondpb.VoluntaryExit{
				ValidatorIndex: 3,
				Epoch:          1,
			},
		},
		{
			Exit: &zondpb.VoluntaryExit{
				ValidatorIndex: 4,
				Epoch:          0,
			},
		},
	}

	block := &zondpb.BeaconBlock{
		Body: &zondpb.BeaconBlockBody{
			VoluntaryExits: exits,
		},
	}

	wb, err := blocks.NewBeaconBlock(block)
	require.NoError(t, err)
	s.processExitsFromBlock(wb)
	require.LogsDoNotContain(t, hook, "\"Voluntary exit was included\"")
}

func TestProcessExitP2PTrackedIndices(t *testing.T) {
	hook := logTest.NewGlobal()
	s := &Service{
		TrackedValidators: map[primitives.ValidatorIndex]bool{
			1: true,
			2: true,
		},
	}

	exit := &zondpb.SignedVoluntaryExit{
		Exit: &zondpb.VoluntaryExit{
			ValidatorIndex: 1,
			Epoch:          1,
		},
		Signature: make([]byte, 96),
	}
	s.processExit(exit)
	require.LogsContain(t, hook, "\"Voluntary exit was processed\" ValidatorIndex=1")
}

func TestProcessExitP2PUntrackedIndices(t *testing.T) {
	hook := logTest.NewGlobal()
	s := &Service{
		TrackedValidators: map[primitives.ValidatorIndex]bool{
			1: true,
			2: true,
		},
	}

	exit := &zondpb.SignedVoluntaryExit{
		Exit: &zondpb.VoluntaryExit{
			ValidatorIndex: 3,
			Epoch:          1,
		},
	}
	s.processExit(exit)
	require.LogsDoNotContain(t, hook, "\"Voluntary exit was processed\"")
}
