package sync

import (
	"io"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/cmd/beacon-chain/flags"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/consensus-types/primitives"
)

// Track attester key lookups to detect signature work before cheap gossip checks.
type pubkeyTrackingState struct {
	state.BeaconState
	pubkeyLookups int
}

func (s *pubkeyTrackingState) PubkeyAtIndex(index primitives.ValidatorIndex) [field_params.MLDSA87PubkeyLength]byte {
	s.pubkeyLookups++
	return s.BeaconState.PubkeyAtIndex(index)
}

func TestMain(m *testing.M) {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetOutput(io.Discard)

	resetFlags := flags.Get()
	flags.Init(&flags.GlobalFlags{
		BlockBatchLimit:            64,
		BlockBatchLimitBurstFactor: 10,
	})
	defer func() {
		flags.Init(resetFlags)
	}()
	m.Run()
}
