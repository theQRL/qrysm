package monitor

import (
	"testing"

	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
)

func TestProcessExitsFromBlockTrackedIndices(t *testing.T) {
	hook := logTest.NewGlobal()
	s := &Service{
		TrackedValidators: map[primitives.ValidatorIndex]bool{
			1: true,
			2: true,
		},
	}

	exits := []*qrysmpb.SignedVoluntaryExit{
		{
			Exit: &qrysmpb.VoluntaryExit{
				ValidatorIndex: 3,
				Epoch:          1,
			},
		},
		{
			Exit: &qrysmpb.VoluntaryExit{
				ValidatorIndex: 2,
				Epoch:          0,
			},
		},
	}

	block := &qrysmpb.BeaconBlockZond{
		Body: &qrysmpb.BeaconBlockBodyZond{
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

	exits := []*qrysmpb.SignedVoluntaryExit{
		{
			Exit: &qrysmpb.VoluntaryExit{
				ValidatorIndex: 3,
				Epoch:          1,
			},
		},
		{
			Exit: &qrysmpb.VoluntaryExit{
				ValidatorIndex: 4,
				Epoch:          0,
			},
		},
	}

	block := &qrysmpb.BeaconBlockZond{
		Body: &qrysmpb.BeaconBlockBodyZond{
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

	exit := &qrysmpb.SignedVoluntaryExit{
		Exit: &qrysmpb.VoluntaryExit{
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

	exit := &qrysmpb.SignedVoluntaryExit{
		Exit: &qrysmpb.VoluntaryExit{
			ValidatorIndex: 3,
			Epoch:          1,
		},
	}
	s.processExit(exit)
	require.LogsDoNotContain(t, hook, "\"Voluntary exit was processed\"")
}
