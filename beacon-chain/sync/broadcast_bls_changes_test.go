package sync

import (
	"context"
	"testing"
	"time"

	logTest "github.com/sirupsen/logrus/hooks/test"
	mockChain "github.com/theQRL/qrysm/v4/beacon-chain/blockchain/testing"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/signing"
	testingdb "github.com/theQRL/qrysm/v4/beacon-chain/db/testing"
	doublylinkedtree "github.com/theQRL/qrysm/v4/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/theQRL/qrysm/v4/beacon-chain/operations/blstoexec"
	mockp2p "github.com/theQRL/qrysm/v4/beacon-chain/p2p/testing"
	"github.com/theQRL/qrysm/v4/beacon-chain/state/stategen"
	mockSync "github.com/theQRL/qrysm/v4/beacon-chain/sync/initial-sync/testing"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/testing/util"
	"github.com/theQRL/qrysm/v4/time/slots"
)

func TestBroadcastDilithiumChanges(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	c := params.BeaconConfig()
	c.CapellaForkEpoch = c.BellatrixForkEpoch.Add(2)
	params.OverrideBeaconConfig(c)
	chainService := &mockChain.ChainService{
		Genesis:        time.Now(),
		ValidatorsRoot: [32]byte{'A'},
	}
	s := NewService(context.Background(),
		WithP2P(mockp2p.NewTestP2P(t)),
		WithInitialSync(&mockSync.Sync{IsSyncing: false}),
		WithChainService(chainService),
		WithOperationNotifier(chainService.OperationNotifier()),
		WithDilithiumToExecPool(blstoexec.NewPool()),
	)
	var emptySig [96]byte
	s.cfg.dilithiumToExecPool.InsertDilithiumToExecChange(&zondpb.SignedDilithiumToExecutionChange{
		Message: &zondpb.DilithiumToExecutionChange{
			ValidatorIndex:      10,
			FromDilithiumPubkey: make([]byte, 48),
			ToExecutionAddress:  make([]byte, 20),
		},
		Signature: emptySig[:],
	})

	capellaStart, err := slots.EpochStart(params.BeaconConfig().CapellaForkEpoch)
	require.NoError(t, err)
	s.broadcastDilithiumChanges(capellaStart + 1)
}

func TestRateDilithiumChanges(t *testing.T) {
	logHook := logTest.NewGlobal()
	params.SetupTestConfigCleanup(t)
	c := params.BeaconConfig()
	c.CapellaForkEpoch = c.BellatrixForkEpoch.Add(2)
	params.OverrideBeaconConfig(c)
	chainService := &mockChain.ChainService{
		Genesis:        time.Now(),
		ValidatorsRoot: [32]byte{'A'},
	}
	p1 := mockp2p.NewTestP2P(t)
	s := NewService(context.Background(),
		WithP2P(p1),
		WithInitialSync(&mockSync.Sync{IsSyncing: false}),
		WithChainService(chainService),
		WithOperationNotifier(chainService.OperationNotifier()),
		WithDilithiumToExecPool(blstoexec.NewPool()),
	)
	beaconDB := testingdb.SetupDB(t)
	s.cfg.stateGen = stategen.New(beaconDB, doublylinkedtree.New())
	s.cfg.beaconDB = beaconDB
	s.initCaches()
	st, keys := util.DeterministicGenesisStateCapella(t, 256)
	s.cfg.chain = &mockChain.ChainService{
		ValidatorsRoot: [32]byte{'A'},
		Genesis:        time.Now().Add(-time.Second * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Duration(10)),
		State:          st,
	}

	for i := 0; i < 200; i++ {
		message := &zondpb.DilithiumToExecutionChange{
			ValidatorIndex:      primitives.ValidatorIndex(i),
			FromDilithiumPubkey: keys[i+1].PublicKey().Marshal(),
			ToExecutionAddress:  bytesutil.PadTo([]byte("address"), 20),
		}
		epoch := params.BeaconConfig().CapellaForkEpoch + 1
		domain, err := signing.Domain(st.Fork(), epoch, params.BeaconConfig().DomainDilithiumToExecutionChange, st.GenesisValidatorsRoot())
		assert.NoError(t, err)
		htr, err := signing.SigningData(message.HashTreeRoot, domain)
		assert.NoError(t, err)
		signed := &zondpb.SignedDilithiumToExecutionChange{
			Message:   message,
			Signature: keys[i+1].Sign(htr[:]).Marshal(),
		}

		s.cfg.dilithiumToExecPool.InsertDilithiumToExecChange(signed)
	}

	require.Equal(t, false, p1.BroadcastCalled)
	slot, err := slots.EpochStart(params.BeaconConfig().CapellaForkEpoch)
	require.NoError(t, err)
	s.broadcastDilithiumChanges(slot)
	time.Sleep(100 * time.Millisecond) // Need a sleep for the go routine to be ready
	require.Equal(t, true, p1.BroadcastCalled)
	require.LogsDoNotContain(t, logHook, "could not")

	p1.BroadcastCalled = false
	time.Sleep(500 * time.Millisecond) // Need a sleep for the second batch to be broadcast
	require.Equal(t, true, p1.BroadcastCalled)
	require.LogsDoNotContain(t, logHook, "could not")
}

func TestBroadcastDilithiumBatch_changes_slice(t *testing.T) {
	message := &zondpb.DilithiumToExecutionChange{
		FromDilithiumPubkey: make([]byte, 48),
		ToExecutionAddress:  make([]byte, 20),
	}
	signed := &zondpb.SignedDilithiumToExecutionChange{
		Message:   message,
		Signature: make([]byte, 96),
	}
	changes := make([]*zondpb.SignedDilithiumToExecutionChange, 200)
	for i := 0; i < len(changes); i++ {
		changes[i] = signed
	}
	p1 := mockp2p.NewTestP2P(t)
	chainService := &mockChain.ChainService{
		Genesis:        time.Now(),
		ValidatorsRoot: [32]byte{'A'},
	}
	s := NewService(context.Background(),
		WithP2P(p1),
		WithInitialSync(&mockSync.Sync{IsSyncing: false}),
		WithChainService(chainService),
		WithOperationNotifier(chainService.OperationNotifier()),
		WithDilithiumToExecPool(blstoexec.NewPool()),
	)
	beaconDB := testingdb.SetupDB(t)
	s.cfg.stateGen = stategen.New(beaconDB, doublylinkedtree.New())
	s.cfg.beaconDB = beaconDB
	s.initCaches()
	st, _ := util.DeterministicGenesisStateCapella(t, 32)
	s.cfg.chain = &mockChain.ChainService{
		ValidatorsRoot: [32]byte{'A'},
		Genesis:        time.Now().Add(-time.Second * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Duration(10)),
		State:          st,
	}

	s.broadcastDilithiumBatch(s.ctx, &changes)
	require.Equal(t, 200-128, len(changes))
}
