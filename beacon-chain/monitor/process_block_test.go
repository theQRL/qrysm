package monitor

import (
	"context"
	"fmt"
	"testing"
	"time"

	logTest "github.com/sirupsen/logrus/hooks/test"
	mock "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	"github.com/theQRL/qrysm/beacon-chain/core/altair"
	testDB "github.com/theQRL/qrysm/beacon-chain/db/testing"
	doublylinkedtree "github.com/theQRL/qrysm/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/beacon-chain/state/stategen"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestProcessSlashings(t *testing.T) {
	tests := []struct {
		name      string
		block     *qrysmpb.BeaconBlockZond
		wantedErr string
	}{
		{
			name: "Proposer slashing a tracked index",
			block: &qrysmpb.BeaconBlockZond{
				Body: &qrysmpb.BeaconBlockBodyZond{
					ProposerSlashings: []*qrysmpb.ProposerSlashing{
						{
							Header_1: &qrysmpb.SignedBeaconBlockHeader{
								Header: &qrysmpb.BeaconBlockHeader{
									ProposerIndex: 2,
									Slot:          params.BeaconConfig().SlotsPerEpoch + 1,
								},
							},
							Header_2: &qrysmpb.SignedBeaconBlockHeader{
								Header: &qrysmpb.BeaconBlockHeader{
									ProposerIndex: 2,
									Slot:          0,
								},
							},
						},
					},
				},
			},
			wantedErr: "\"Proposer slashing was included\" BodyRoot1= BodyRoot2= ProposerIndex=2",
		},
		{
			name: "Proposer slashing an untracked index",
			block: &qrysmpb.BeaconBlockZond{
				Body: &qrysmpb.BeaconBlockBodyZond{
					ProposerSlashings: []*qrysmpb.ProposerSlashing{
						{
							Header_1: &qrysmpb.SignedBeaconBlockHeader{
								Header: &qrysmpb.BeaconBlockHeader{
									ProposerIndex: 3,
									Slot:          params.BeaconConfig().SlotsPerEpoch + 4,
								},
							},
							Header_2: &qrysmpb.SignedBeaconBlockHeader{
								Header: &qrysmpb.BeaconBlockHeader{
									ProposerIndex: 3,
									Slot:          0,
								},
							},
						},
					},
				},
			},
			wantedErr: "",
		},
		{
			name: "Attester slashing a tracked index",
			block: &qrysmpb.BeaconBlockZond{
				Body: &qrysmpb.BeaconBlockBodyZond{
					AttesterSlashings: []*qrysmpb.AttesterSlashing{
						{
							Attestation_1: util.HydrateIndexedAttestation(&qrysmpb.IndexedAttestation{
								Data: &qrysmpb.AttestationData{
									Source: &qrysmpb.Checkpoint{Epoch: 1},
								},
								AttestingIndices: []uint64{1, 3, 4},
							}),
							Attestation_2: util.HydrateIndexedAttestation(&qrysmpb.IndexedAttestation{
								AttestingIndices: []uint64{1, 5, 6},
							}),
						},
					},
				},
			},
			wantedErr: "\"Attester slashing was included\" AttestationSlot1=0 AttestationSlot2=0 AttesterIndex=1 " +
				"BeaconBlockRoot1=0x000000000000 BeaconBlockRoot2=0x000000000000 BlockInclusionSlot=0 SourceEpoch1=1 SourceEpoch2=0 TargetEpoch1=0 TargetEpoch2=0",
		},
		{
			name: "Attester slashing untracked index",
			block: &qrysmpb.BeaconBlockZond{
				Body: &qrysmpb.BeaconBlockBodyZond{
					AttesterSlashings: []*qrysmpb.AttesterSlashing{
						{
							Attestation_1: util.HydrateIndexedAttestation(&qrysmpb.IndexedAttestation{
								Data: &qrysmpb.AttestationData{
									Source: &qrysmpb.Checkpoint{Epoch: 1},
								},
								AttestingIndices: []uint64{1, 3, 4},
							}),
							Attestation_2: util.HydrateIndexedAttestation(&qrysmpb.IndexedAttestation{
								AttestingIndices: []uint64{3, 5, 6},
							}),
						},
					},
				},
			},
			wantedErr: "",
		}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hook := logTest.NewGlobal()
			s := &Service{
				TrackedValidators: map[primitives.ValidatorIndex]bool{
					1: true,
					2: true,
				},
			}
			wb, err := blocks.NewBeaconBlock(tt.block)
			require.NoError(t, err)
			s.processSlashings(wb)
			if tt.wantedErr != "" {
				require.LogsContain(t, hook, tt.wantedErr)
			} else {
				require.LogsDoNotContain(t, hook, "slashing")
			}
		})
	}
}

func TestProcessProposedBlock(t *testing.T) {
	tests := []struct {
		name      string
		block     *qrysmpb.BeaconBlockZond
		wantedErr string
	}{
		{
			name: "Block proposed by tracked validator",
			block: &qrysmpb.BeaconBlockZond{
				Slot:          6,
				ProposerIndex: 44,
				ParentRoot:    bytesutil.PadTo([]byte("hello-world"), 32),
				StateRoot:     bytesutil.PadTo([]byte("state-world"), 32),
				Body:          &qrysmpb.BeaconBlockBodyZond{},
			},
			wantedErr: "\"Proposed beacon block was included\" BalanceChange=100000000 BlockRoot=0x68656c6c6f2d NewBalance=40000000000000 ParentRoot=0x68656c6c6f2d ProposerIndex=44 Slot=6 Version=0 prefix=monitor",
		},
		{
			name: "Block proposed by untracked validator",
			block: &qrysmpb.BeaconBlockZond{
				Slot:          6,
				ProposerIndex: 13,
				ParentRoot:    bytesutil.PadTo([]byte("hello-world"), 32),
				StateRoot:     bytesutil.PadTo([]byte("state-world"), 32),
				Body:          &qrysmpb.BeaconBlockBodyZond{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hook := logTest.NewGlobal()
			s := setupService(t)
			beaconState, _ := util.DeterministicGenesisStateZond(t, 256)
			var root [32]byte
			copy(root[:], "hello-world")
			wb, err := blocks.NewBeaconBlock(tt.block)
			require.NoError(t, err)
			s.processProposedBlock(beaconState, root, wb)
			if tt.wantedErr != "" {
				require.LogsContain(t, hook, tt.wantedErr)
			} else {
				require.LogsDoNotContain(t, hook, "included")
			}
		})
	}

}

type resyncMonitoringStateManager struct {
	stategen.StateManager
	afterRead func([32]byte)
}

func (m *resyncMonitoringStateManager) StateByRootIfCached(root [32]byte) state.BeaconState {
	st := m.StateManager.StateByRootIfCached(root)
	if st != nil && m.afterRead != nil {
		m.afterRead(root)
	}
	return st
}

func TestProcessBlock_StateSnapshotDuringResync(t *testing.T) {
	for _, tc := range []struct {
		name   string
		cached bool
		resync bool
	}{
		{name: "cached state control", cached: true},
		{name: "resync consumes cached state", cached: true, resync: true},
		{name: "cache miss"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			st, err := util.NewBeaconStateZond()
			require.NoError(t, err)
			require.NoError(t, st.SetSlot(1))
			require.NoError(t, st.SetBalances([]uint64{10}))
			b := util.NewBeaconBlockZond()
			b.Block.Slot = 1
			wrapped, err := blocks.NewSignedBeaconBlock(b)
			require.NoError(t, err)
			root, err := wrapped.Block().HashTreeRoot()
			require.NoError(t, err)
			manager := stategen.New(testDB.SetupDB(t), doublylinkedtree.New())
			if tc.cached {
				require.NoError(t, manager.SaveState(ctx, root, st))
			}
			resynced := false
			reader := &resyncMonitoringStateManager{StateManager: manager}
			if tc.resync {
				reader.afterRead = func(r [32]byte) {
					// A resync batch takes ownership of the cached parent after
					// monitoring retrieves it, before monitoring reads balances.
					next, err := manager.StateByRootInitialSync(ctx, r)
					require.NoError(t, err)
					require.NoError(t, next.SetSlot(2))
					require.NoError(t, next.UpdateBalancesAtIndex(0, 20))
					resynced = true
				}
			}
			svc := &Service{
				config:                &ValidatorMonitorConfig{StateGen: reader},
				TrackedValidators:     map[primitives.ValidatorIndex]bool{0: true},
				latestPerformance:     map[primitives.ValidatorIndex]ValidatorLatestPerformance{0: {balance: 9}},
				aggregatedPerformance: make(map[primitives.ValidatorIndex]ValidatorAggregatedPerformance),
			}
			svc.processBlock(ctx, wrapped)
			require.Equal(t, tc.resync, resynced)
			if tc.cached {
				require.Equal(t, uint64(1), svc.aggregatedPerformance[0].totalProposedCount)
				require.Equal(t, uint64(10), svc.latestPerformance[0].balance, "monitor the imported block's state, not the next batch's working state")
			} else {
				require.Equal(t, uint64(0), svc.aggregatedPerformance[0].totalProposedCount)
				require.Equal(t, uint64(9), svc.latestPerformance[0].balance)
			}
		})
	}
}

func TestProcessBlock_AllEventsTrackedVals(t *testing.T) {
	hook := logTest.NewGlobal()
	ctx := context.Background()

	genesis, keys := util.DeterministicGenesisStateZond(t, 256)
	c, err := altair.NextSyncCommittee(ctx, genesis)
	require.NoError(t, err)
	require.NoError(t, genesis.SetCurrentSyncCommittee(c))

	genConfig := util.DefaultBlockGenConfig()
	genConfig.NumProposerSlashings = 1
	genConfig.FullSyncAggregate = true
	b, err := util.GenerateFullBlockZond(genesis, keys, genConfig, 1)
	require.NoError(t, err)

	beaconDB := testDB.SetupDB(t)

	chainService := &mock.ChainService{
		Genesis:        time.Now(),
		DB:             beaconDB,
		State:          genesis,
		Root:           []byte("hello-world"),
		ValidatorsRoot: [32]byte{},
	}

	blockProposerIndex := b.Block.ProposerIndex
	trackedVals := map[primitives.ValidatorIndex]bool{
		blockProposerIndex: true,
		1:                  true,
		2:                  true,
	}

	latestPerformance := map[primitives.ValidatorIndex]ValidatorLatestPerformance{
		blockProposerIndex: {
			balance: 39999900000000,
		},
		1: {
			balance: 40000000000000,
		},
		2: {
			balance: 40000000000000,
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

		ctx:                         context.Background(),
		TrackedValidators:           trackedVals,
		latestPerformance:           latestPerformance,
		aggregatedPerformance:       make(map[primitives.ValidatorIndex]ValidatorAggregatedPerformance),
		trackedSyncCommitteeIndices: make(map[primitives.ValidatorIndex][]primitives.CommitteeIndex),
		lastSyncedEpoch:             0,
	}

	pubKeys := make([][]byte, 3)
	pubKeys[0] = genesis.Validators()[0].PublicKey
	pubKeys[1] = genesis.Validators()[1].PublicKey
	pubKeys[2] = genesis.Validators()[2].PublicKey

	currentSyncCommittee := util.ConvertToCommittee([][]byte{
		pubKeys[0], pubKeys[1], pubKeys[2], pubKeys[1], pubKeys[1],
	})
	require.NoError(t, genesis.SetCurrentSyncCommittee(currentSyncCommittee))

	idx := b.Block.Body.ProposerSlashings[0].Header_1.Header.ProposerIndex
	svc.RLock()
	if !svc.trackedIndex(idx) {
		svc.TrackedValidators[idx] = true
		svc.latestPerformance[idx] = ValidatorLatestPerformance{
			balance: 39999900000000,
		}
		svc.aggregatedPerformance[idx] = ValidatorAggregatedPerformance{}
	}
	svc.RUnlock()
	svc.updateSyncCommitteeTrackedVals(genesis)

	root, err := b.GetBlock().HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, svc.config.StateGen.SaveState(ctx, root, genesis))
	wanted1 := fmt.Sprintf("\"Proposed beacon block was included\" BalanceChange=100000000 BlockRoot=%#x NewBalance=40000000000000 ParentRoot=%#x ProposerIndex=%d Slot=1 Version=0 prefix=monitor", bytesutil.Trunc(root[:]), bytesutil.Trunc(b.Block.ParentRoot), blockProposerIndex)
	wanted2 := fmt.Sprintf("\"Proposer slashing was included\" BodyRoot1=0x000100000000 BodyRoot2=0x000200000000 ProposerIndex=%d SlashingSlot=0 Slot=1 prefix=monitor", idx)
	wanted3 := "\"Sync committee contribution included\" BalanceChange=0 ContribCount=3 ExpectedContribCount=3 NewBalance=40000000000000 ValidatorIndex=1 prefix=monitor"
	wanted4 := "\"Sync committee contribution included\" BalanceChange=0 ContribCount=1 ExpectedContribCount=1 NewBalance=40000000000000 ValidatorIndex=2 prefix=monitor"
	wrapped, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	svc.processBlock(ctx, wrapped)
	require.LogsContain(t, hook, wanted1)
	require.LogsContain(t, hook, wanted2)
	require.LogsContain(t, hook, wanted3)
	require.LogsContain(t, hook, wanted4)
}

// NOTE(rgeraldes24): the original test is not ok since the map iteration is not ordered and the output
// will not be printed if any key other than 1(aggregatedPerformance map) is processed first
func TestLogAggregatedPerformance(t *testing.T) {
	hook := logTest.NewGlobal()
	latestPerformance := map[primitives.ValidatorIndex]ValidatorLatestPerformance{
		1: {
			balance: 40000000000000,
		},
		// 107: {
		// 	balance: 40000000000000,
		// },
		// 86: {
		// 	balance: 39999900000000,
		// },
		// 15: {
		// 	balance: 39999900000000,
		// },
	}
	aggregatedPerformance := map[primitives.ValidatorIndex]ValidatorAggregatedPerformance{
		1: {
			startEpoch:                      0,
			startBalance:                    39625000000000,
			totalAttestedCount:              12,
			totalRequestedCount:             15,
			totalDistance:                   14,
			totalCorrectHead:                8,
			totalCorrectSource:              11,
			totalCorrectTarget:              12,
			totalProposedCount:              1,
			totalSyncCommitteeContributions: 0,
			totalSyncCommitteeAggregations:  0,
		},
		// 107: {},
		// 86:  {},
		// 15:  {},
	}
	s := &Service{
		latestPerformance:     latestPerformance,
		aggregatedPerformance: aggregatedPerformance,
	}

	s.logAggregatedPerformance()
	wanted := "\"Aggregated performance since launch\" AttestationInclusion=\"80.00%\"" +
		" AverageInclusionDistance=1.2 BalanceChangePct=\"0.95%\" CorrectlyVotedHeadPct=\"66.67%\" " +
		"CorrectlyVotedSourcePct=\"91.67%\" CorrectlyVotedTargetPct=\"100.00%\" StartBalance=39625000000000 " +
		"StartEpoch=0 TotalAggregations=0 TotalProposedBlocks=1 TotalRequested=15 TotalSyncContributions=0 " +
		"ValidatorIndex=1 prefix=monitor"
	require.LogsContain(t, hook, wanted)
}
