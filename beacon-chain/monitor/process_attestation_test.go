package monitor

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/theQRL/go-bitfield"
	mock "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	testDB "github.com/theQRL/qrysm/beacon-chain/db/testing"
	doublylinkedtree "github.com/theQRL/qrysm/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/theQRL/qrysm/beacon-chain/state/stategen"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestGetAttestingIndices(t *testing.T) {
	ctx := context.Background()
	beaconState, _ := util.DeterministicGenesisStateZond(t, 256)
	att := &qrysmpb.Attestation{
		Data: &qrysmpb.AttestationData{
			Slot:           1,
			CommitteeIndex: 0,
		},
		AggregationBits: bitfield.Bitlist{0b111},
	}
	attestingIndices, err := attestingIndices(ctx, beaconState, att)
	require.NoError(t, err)
	require.Equal(t, 2, len(attestingIndices))

}

func trackAttestingIndices(t *testing.T, s *Service, indices []uint64) {
	require.Equal(t, true, len(indices) > 0)
	s.TrackedValidators = map[primitives.ValidatorIndex]bool{}
	s.latestPerformance = map[primitives.ValidatorIndex]ValidatorLatestPerformance{}
	s.aggregatedPerformance = map[primitives.ValidatorIndex]ValidatorAggregatedPerformance{}
	for i, rawIndex := range indices {
		index := primitives.ValidatorIndex(rawIndex)
		s.TrackedValidators[index] = true
		s.aggregatedPerformance[index] = ValidatorAggregatedPerformance{}
		balance := uint64(40000000000000)
		if i == 1 {
			balance = 39999900000000
		}
		s.latestPerformance[index] = ValidatorLatestPerformance{
			balance:      balance,
			timelyHead:   true,
			timelySource: true,
			timelyTarget: true,
		}
	}
}

func TestProcessIncludedAttestationTwoTracked(t *testing.T) {
	hook := logTest.NewGlobal()
	s := setupService(t)
	state, _ := util.DeterministicGenesisStateZond(t, 256)
	require.NoError(t, state.SetSlot(2))
	require.NoError(t, state.SetCurrentParticipationBits(bytes.Repeat([]byte{0xff}, 13)))

	att := &qrysmpb.Attestation{
		Data: &qrysmpb.AttestationData{
			Slot:            1,
			CommitteeIndex:  0,
			BeaconBlockRoot: bytesutil.PadTo([]byte("hello-world"), 32),
			Source: &qrysmpb.Checkpoint{
				Epoch: 0,
				Root:  bytesutil.PadTo([]byte("hello-world"), 32),
			},
			Target: &qrysmpb.Checkpoint{
				Epoch: 1,
				Root:  bytesutil.PadTo([]byte("hello-world"), 32),
			},
		},
		AggregationBits: bitfield.Bitlist{0b111},
	}
	indices, err := attestingIndices(context.Background(), state, att)
	require.NoError(t, err)
	trackAttestingIndices(t, s, indices)

	s.processIncludedAttestation(context.Background(), state, att)
	wanted1 := fmt.Sprintf("\"Attestation included\" BalanceChange=0 CorrectHead=true CorrectSource=true CorrectTarget=true Head=0x68656c6c6f2d InclusionSlot=2 NewBalance=40000000000000 Slot=1 Source=0x68656c6c6f2d Target=0x68656c6c6f2d ValidatorIndex=%d prefix=monitor", indices[0])
	wanted2 := fmt.Sprintf("\"Attestation included\" BalanceChange=100000000 CorrectHead=true CorrectSource=true CorrectTarget=true Head=0x68656c6c6f2d InclusionSlot=2 NewBalance=40000000000000 Slot=1 Source=0x68656c6c6f2d Target=0x68656c6c6f2d ValidatorIndex=%d prefix=monitor", indices[1])
	require.LogsContain(t, hook, wanted1)
	require.LogsContain(t, hook, wanted2)
}

func TestProcessUnaggregatedAttestationStateNotCached(t *testing.T) {
	logrus.SetLevel(logrus.DebugLevel)
	hook := logTest.NewGlobal()
	ctx := context.Background()

	s := setupService(t)
	state, _ := util.DeterministicGenesisStateZond(t, 256)
	require.NoError(t, state.SetSlot(2))
	header := state.LatestBlockHeader()
	participation := []byte{0xff, 0xff, 0x01, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	require.NoError(t, state.SetCurrentParticipationBits(participation))

	att := &qrysmpb.Attestation{
		Data: &qrysmpb.AttestationData{
			Slot:            1,
			CommitteeIndex:  0,
			BeaconBlockRoot: header.GetStateRoot(),
			Source: &qrysmpb.Checkpoint{
				Epoch: 0,
				Root:  bytesutil.PadTo([]byte("hello-world"), 32),
			},
			Target: &qrysmpb.Checkpoint{
				Epoch: 1,
				Root:  bytesutil.PadTo([]byte("hello-world"), 32),
			},
		},
		AggregationBits: bitfield.Bitlist{0b11, 0b1},
	}
	s.processUnaggregatedAttestation(ctx, att)
	require.LogsContain(t, hook, "Skipping unaggregated attestation due to state not found in cache")
	logrus.SetLevel(logrus.InfoLevel)
}

func TestProcessUnaggregatedAttestationStateCached(t *testing.T) {
	ctx := context.Background()
	hook := logTest.NewGlobal()

	s := setupService(t)
	state, _ := util.DeterministicGenesisStateZond(t, 256)
	participation := []byte{0xff, 0xff, 0x01}
	require.NoError(t, state.SetCurrentParticipationBits(participation))

	var root [32]byte
	copy(root[:], "hello-world")

	att := &qrysmpb.Attestation{
		Data: &qrysmpb.AttestationData{
			Slot:            1,
			CommitteeIndex:  0,
			BeaconBlockRoot: root[:],
			Source: &qrysmpb.Checkpoint{
				Epoch: 0,
				Root:  root[:],
			},
			Target: &qrysmpb.Checkpoint{
				Epoch: 1,
				Root:  root[:],
			},
		},
		AggregationBits: bitfield.Bitlist{0b111},
	}
	indices, err := attestingIndices(ctx, state, att)
	require.NoError(t, err)
	trackAttestingIndices(t, s, indices)
	require.NoError(t, s.config.StateGen.SaveState(ctx, root, state))
	s.processUnaggregatedAttestation(context.Background(), att)
	wanted1 := fmt.Sprintf("\"Processed unaggregated attestation\" Head=0x68656c6c6f2d Slot=1 Source=0x68656c6c6f2d Target=0x68656c6c6f2d ValidatorIndex=%d prefix=monitor", indices[0])
	wanted2 := fmt.Sprintf("\"Processed unaggregated attestation\" Head=0x68656c6c6f2d Slot=1 Source=0x68656c6c6f2d Target=0x68656c6c6f2d ValidatorIndex=%d prefix=monitor", indices[1])
	require.LogsContain(t, hook, wanted1)
	require.LogsContain(t, hook, wanted2)
}

func TestProcessAggregatedAttestationStateNotCached(t *testing.T) {
	logrus.SetLevel(logrus.DebugLevel)
	hook := logTest.NewGlobal()
	ctx := context.Background()

	state, _ := util.DeterministicGenesisStateZond(t, 256)
	beaconDB := testDB.SetupDB(t)

	chainService := &mock.ChainService{
		Genesis:        time.Now(),
		DB:             beaconDB,
		State:          state,
		Root:           []byte("hello-world"),
		ValidatorsRoot: [32]byte{},
	}
	aggregatedPerformance := map[primitives.ValidatorIndex]ValidatorAggregatedPerformance{
		44: {},
	}
	trackedVals := map[primitives.ValidatorIndex]bool{
		44: true,
	}
	latestPerformance := map[primitives.ValidatorIndex]ValidatorLatestPerformance{
		44: {
			balance:      39999900000000,
			timelyHead:   true,
			timelySource: true,
			timelyTarget: true,
		},
	}

	svc := &Service{
		config: &ValidatorMonitorConfig{
			StateGen:            stategen.New(beaconDB, doublylinkedtree.New()),
			StateNotifier:       chainService.StateNotifier(),
			HeadFetcher:         chainService,
			AttestationNotifier: chainService.OperationNotifier(),
			InitialSyncComplete: make(chan struct{}),
		},
		ctx:                   context.Background(),
		TrackedValidators:     trackedVals,
		latestPerformance:     latestPerformance,
		aggregatedPerformance: aggregatedPerformance,
		lastSyncedEpoch:       0,
	}

	require.NoError(t, state.SetSlot(2))
	header := state.LatestBlockHeader()
	participation := []byte{0xff, 0xff, 0x01}
	require.NoError(t, state.SetCurrentParticipationBits(participation))

	att := &qrysmpb.AggregateAttestationAndProof{
		AggregatorIndex: 44,
		Aggregate: &qrysmpb.Attestation{
			Data: &qrysmpb.AttestationData{
				Slot:            1,
				CommitteeIndex:  0,
				BeaconBlockRoot: header.GetStateRoot(),
				Source: &qrysmpb.Checkpoint{
					Epoch: 0,
					Root:  bytesutil.PadTo([]byte("hello-world"), 32),
				},
				Target: &qrysmpb.Checkpoint{
					Epoch: 1,
					Root:  bytesutil.PadTo([]byte("hello-world"), 32),
				},
			},
			AggregationBits: bitfield.Bitlist{0b111},
		},
	}
	svc.processAggregatedAttestation(ctx, att)
	require.LogsContain(t, hook, "\"Processed attestation aggregation\" AggregatorIndex=44 BeaconBlockRoot=0x000000000000 Slot=1 SourceRoot=0x68656c6c6f2d TargetRoot=0x68656c6c6f2d prefix=monitor")
	require.LogsContain(t, hook, "Skipping aggregated attestation due to state not found in cache")
	logrus.SetLevel(logrus.InfoLevel)
}

func TestProcessAggregatedAttestationStateCached(t *testing.T) {
	hook := logTest.NewGlobal()
	ctx := context.Background()

	beaconDB := testDB.SetupDB(t)
	state, _ := util.DeterministicGenesisStateZond(t, 256)

	chainService := &mock.ChainService{
		Genesis:        time.Now(),
		DB:             beaconDB,
		State:          state,
		Root:           []byte("hello-world"),
		ValidatorsRoot: [32]byte{},
	}
	aggregatedPerformance := map[primitives.ValidatorIndex]ValidatorAggregatedPerformance{
		166: {},
		117: {},
		59:  {},
	}
	trackedVals := map[primitives.ValidatorIndex]bool{
		166: true,
		117: true,
		59:  true,
	}
	latestPerformance := map[primitives.ValidatorIndex]ValidatorLatestPerformance{
		166: {
			balance:      39999900000000,
			timelyHead:   true,
			timelySource: true,
			timelyTarget: true,
		},
		117: {
			balance:      40000000000000,
			timelyHead:   true,
			timelySource: true,
			timelyTarget: true,
		},
		59: {
			balance:      39999900000000,
			timelyHead:   true,
			timelySource: true,
			timelyTarget: true,
		},
	}

	svc := &Service{
		config: &ValidatorMonitorConfig{
			StateGen:            stategen.New(beaconDB, doublylinkedtree.New()),
			StateNotifier:       chainService.StateNotifier(),
			HeadFetcher:         chainService,
			AttestationNotifier: chainService.OperationNotifier(),
			InitialSyncComplete: make(chan struct{}),
		},
		ctx:                   context.Background(),
		TrackedValidators:     trackedVals,
		latestPerformance:     latestPerformance,
		aggregatedPerformance: aggregatedPerformance,
		lastSyncedEpoch:       0,
	}

	participation := []byte{0xff, 0xff, 0x01}
	require.NoError(t, state.SetCurrentParticipationBits(participation))

	var root [32]byte
	copy(root[:], "hello-world")

	att := &qrysmpb.AggregateAttestationAndProof{
		AggregatorIndex: 117,
		Aggregate: &qrysmpb.Attestation{
			Data: &qrysmpb.AttestationData{
				Slot:            1,
				CommitteeIndex:  0,
				BeaconBlockRoot: root[:],
				Source: &qrysmpb.Checkpoint{
					Epoch: 0,
					Root:  root[:],
				},
				Target: &qrysmpb.Checkpoint{
					Epoch: 1,
					Root:  root[:],
				},
			},
			AggregationBits: bitfield.Bitlist{0b101},
		},
	}

	require.NoError(t, svc.config.StateGen.SaveState(ctx, root, state))
	indices, err := attestingIndices(ctx, state, att.Aggregate)
	require.NoError(t, err)
	trackAttestingIndices(t, svc, indices)
	aggregatorIndex := primitives.ValidatorIndex(att.AggregatorIndex)
	svc.TrackedValidators[aggregatorIndex] = true
	svc.aggregatedPerformance[aggregatorIndex] = ValidatorAggregatedPerformance{}
	svc.latestPerformance[aggregatorIndex] = ValidatorLatestPerformance{
		balance:      40000000000000,
		timelyHead:   true,
		timelySource: true,
		timelyTarget: true,
	}
	svc.processAggregatedAttestation(ctx, att)
	require.LogsContain(t, hook, "\"Processed attestation aggregation\" AggregatorIndex=117 BeaconBlockRoot=0x68656c6c6f2d Slot=1 SourceRoot=0x68656c6c6f2d TargetRoot=0x68656c6c6f2d prefix=monitor")
	require.LogsContain(t, hook, fmt.Sprintf("\"Processed aggregated attestation\" Head=0x68656c6c6f2d Slot=1 Source=0x68656c6c6f2d Target=0x68656c6c6f2d ValidatorIndex=%d prefix=monitor", indices[0]))
	if len(indices) > 1 {
		require.LogsDoNotContain(t, hook, fmt.Sprintf("\"Processed aggregated attestation\" Head=0x68656c6c6f2d Slot=1 Source=0x68656c6c6f2d Target=0x68656c6c6f2d ValidatorIndex=%d prefix=monitor", indices[1]))
	}
}

func TestProcessAttestations(t *testing.T) {
	hook := logTest.NewGlobal()
	s := setupService(t)

	ctx := context.Background()
	state, _ := util.DeterministicGenesisStateZond(t, 256)
	require.NoError(t, state.SetSlot(2))
	require.NoError(t, state.SetCurrentParticipationBits(bytes.Repeat([]byte{0xff}, 13)))

	att := &qrysmpb.Attestation{
		Data: &qrysmpb.AttestationData{
			Slot:            1,
			CommitteeIndex:  0,
			BeaconBlockRoot: bytesutil.PadTo([]byte("hello-world"), 32),
			Source: &qrysmpb.Checkpoint{
				Epoch: 0,
				Root:  bytesutil.PadTo([]byte("hello-world"), 32),
			},
			Target: &qrysmpb.Checkpoint{
				Epoch: 1,
				Root:  bytesutil.PadTo([]byte("hello-world"), 32),
			},
		},
		AggregationBits: bitfield.Bitlist{0b111},
	}

	block := &qrysmpb.BeaconBlockZond{
		Slot: 2,
		Body: &qrysmpb.BeaconBlockBodyZond{
			Attestations: []*qrysmpb.Attestation{att},
		},
	}

	wrappedBlock, err := blocks.NewBeaconBlock(block)
	require.NoError(t, err)
	indices, err := attestingIndices(ctx, state, att)
	require.NoError(t, err)
	trackAttestingIndices(t, s, indices)
	s.processAttestations(ctx, state, wrappedBlock)
	wanted1 := fmt.Sprintf("\"Attestation included\" BalanceChange=0 CorrectHead=true CorrectSource=true CorrectTarget=true Head=0x68656c6c6f2d InclusionSlot=2 NewBalance=40000000000000 Slot=1 Source=0x68656c6c6f2d Target=0x68656c6c6f2d ValidatorIndex=%d prefix=monitor", indices[0])
	wanted2 := fmt.Sprintf("\"Attestation included\" BalanceChange=100000000 CorrectHead=true CorrectSource=true CorrectTarget=true Head=0x68656c6c6f2d InclusionSlot=2 NewBalance=40000000000000 Slot=1 Source=0x68656c6c6f2d Target=0x68656c6c6f2d ValidatorIndex=%d prefix=monitor", indices[1])
	require.LogsContain(t, hook, wanted1)
	require.LogsContain(t, hook, wanted2)

}
