package p2p

import (
	"time"

	"github.com/theQRL/qrysm/v4/beacon-chain/forkchoice"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/time/slots"
)

type mockChain struct {
	currentFork     *zondpb.Fork
	genesisValsRoot [32]byte
	genesisTime     time.Time
}

func (m *mockChain) ForkChoicer() forkchoice.ForkChoicer {
	return nil
}

func (m *mockChain) CurrentFork() *zondpb.Fork {
	return m.currentFork
}

func (m *mockChain) GenesisValidatorsRoot() [32]byte {
	return m.genesisValsRoot
}

func (m *mockChain) GenesisTime() time.Time {
	return m.genesisTime
}

func (m *mockChain) CurrentSlot() primitives.Slot {
	return slots.SinceGenesis(m.genesisTime)
}
