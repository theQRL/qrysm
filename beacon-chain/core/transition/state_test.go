package transition_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	"github.com/theQRL/qrysm/beacon-chain/state"
	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	"github.com/theQRL/qrysm/config/features"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/container/trie"
	"github.com/theQRL/qrysm/crypto/hash"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	"google.golang.org/protobuf/proto"
)

func TestGenesisBeaconState_OK(t *testing.T) {
	genesisEpoch := primitives.Epoch(0)

	assert.DeepEqual(t, []byte{0, 0, 0, 0}, params.BeaconConfig().GenesisForkVersion, "GenesisSlot( should be {0,0,0,0} for these tests to pass")
	genesisForkVersion := params.BeaconConfig().GenesisForkVersion

	assert.Equal(t, [32]byte{}, params.BeaconConfig().ZeroHash, "ZeroHash should be all 0s for these tests to pass")
	assert.Equal(t, primitives.Epoch(65536), params.BeaconConfig().EpochsPerHistoricalVector, "EpochsPerHistoricalVector should be 8192 for these tests to pass")

	latestRandaoMixesLength := params.BeaconConfig().EpochsPerHistoricalVector
	assert.Equal(t, uint64(16777216), params.BeaconConfig().HistoricalRootsLimit, "HistoricalRootsLimit should be 16777216 for these tests to pass")

	depositsForChainStart := 100
	assert.Equal(t, primitives.Epoch(512), params.BeaconConfig().EpochsPerSlashingsVector, "EpochsPerSlashingsVector should be 512 for these tests to pass")

	genesisTime := uint64(99999)
	deposits, _, err := util.DeterministicDepositsAndKeys(uint64(depositsForChainStart))
	require.NoError(t, err)
	executionData, err := util.DeterministicExecutionData(len(deposits))
	require.NoError(t, err)
	newState, err := transition.GenesisBeaconStateZond(context.Background(), deposits, genesisTime, executionData, &enginev1.ExecutionPayloadZond{})
	require.NoError(t, err, "Could not execute GenesisBeaconState")

	// Misc fields checks.
	assert.Equal(t, primitives.Slot(0), newState.Slot(), "Slot was not correctly initialized")
	if !proto.Equal(newState.Fork(), &qrysmpb.Fork{
		PreviousVersion: genesisForkVersion,
		CurrentVersion:  genesisForkVersion,
		Epoch:           genesisEpoch,
	}) {
		t.Error("Fork was not correctly initialized")
	}

	// Validator registry fields checks.
	assert.Equal(t, depositsForChainStart, len(newState.Validators()), "Validators was not correctly initialized")
	// Per-validator arrays must be sized like the registry: attestation
	// processing indexes them by validator index.
	prevParticipation, err := newState.PreviousEpochParticipation()
	require.NoError(t, err)
	assert.Equal(t, depositsForChainStart, len(prevParticipation), "PreviousEpochParticipation was not sized to the validator registry")
	currParticipation, err := newState.CurrentEpochParticipation()
	require.NoError(t, err)
	assert.Equal(t, depositsForChainStart, len(currParticipation), "CurrentEpochParticipation was not sized to the validator registry")
	scores, err := newState.InactivityScores()
	require.NoError(t, err)
	assert.Equal(t, depositsForChainStart, len(scores), "InactivityScores was not sized to the validator registry")
	v, err := newState.ValidatorAtIndex(0)
	require.NoError(t, err)
	assert.Equal(t, primitives.Epoch(0), v.ActivationEpoch, "Validators was not correctly initialized")
	v, err = newState.ValidatorAtIndex(0)
	require.NoError(t, err)
	assert.Equal(t, primitives.Epoch(0), v.ActivationEligibilityEpoch, "Validators was not correctly initialized")
	assert.Equal(t, depositsForChainStart, len(newState.Balances()), "Balances was not correctly initialized")

	// Randomness and committees fields checks.
	assert.Equal(t, latestRandaoMixesLength, primitives.Epoch(len(newState.RandaoMixes())), "Length of RandaoMixes was not correctly initialized")
	mix, err := newState.RandaoMixAtIndex(0)
	require.NoError(t, err)
	assert.DeepEqual(t, executionData.BlockHash, mix, "RandaoMixes was not correctly initialized")

	// Finality fields checks.
	assert.Equal(t, genesisEpoch, newState.PreviousJustifiedCheckpoint().Epoch, "PreviousJustifiedCheckpoint.Epoch was not correctly initialized")
	assert.Equal(t, genesisEpoch, newState.CurrentJustifiedCheckpoint().Epoch, "JustifiedEpoch was not correctly initialized")
	assert.Equal(t, genesisEpoch, newState.FinalizedCheckpointEpoch(), "FinalizedSlot was not correctly initialized")
	assert.Equal(t, uint8(0x00), newState.JustificationBits()[0], "JustificationBits was not correctly initialized")

	// Recent state checks.
	assert.DeepEqual(t, make([]uint64, params.BeaconConfig().EpochsPerSlashingsVector), newState.Slashings(), "Slashings was not correctly initialized")

	zeroHash := params.BeaconConfig().ZeroHash[:]
	// History root checks.
	assert.DeepEqual(t, zeroHash, newState.StateRoots()[0], "StateRoots was not correctly initialized")
	assert.DeepEqual(t, zeroHash, newState.BlockRoots()[0], "BlockRoots was not correctly initialized")

	// Deposit root checks.
	assert.DeepEqual(t, executionData.DepositRoot, newState.ExecutionData().DepositRoot, "ExecutionData DepositRoot was not correctly initialized")
	assert.DeepSSZEqual(t, []*qrysmpb.ExecutionData{}, newState.ExecutionDataVotes(), "ExecutionDataVotes was not correctly initialized")
}

func TestGenesisState_HashEquality(t *testing.T) {
	deposits, _, err := util.DeterministicDepositsAndKeys(100)
	require.NoError(t, err)
	ee1 := &enginev1.ExecutionPayloadZond{
		ParentHash:    make([]byte, 32),
		FeeRecipient:  make([]byte, fieldparams.FeeRecipientLength),
		StateRoot:     make([]byte, 32),
		ReceiptsRoot:  make([]byte, 32),
		LogsBloom:     make([]byte, 256),
		PrevRandao:    make([]byte, 32),
		BaseFeePerGas: make([]byte, 32),
		BlockHash:     make([]byte, 32),
	}
	state1, err := transition.GenesisBeaconStateZond(context.Background(), deposits, 0, &qrysmpb.ExecutionData{BlockHash: make([]byte, 32)}, ee1)
	require.NoError(t, err)
	ee := &enginev1.ExecutionPayloadZond{
		ParentHash:    make([]byte, 32),
		FeeRecipient:  make([]byte, fieldparams.FeeRecipientLength),
		StateRoot:     make([]byte, 32),
		ReceiptsRoot:  make([]byte, 32),
		LogsBloom:     make([]byte, 256),
		PrevRandao:    make([]byte, 32),
		BaseFeePerGas: make([]byte, 32),
		BlockHash:     make([]byte, 32),
	}
	state, err := transition.GenesisBeaconStateZond(context.Background(), deposits, 0, &qrysmpb.ExecutionData{BlockHash: make([]byte, 32)}, ee)
	require.NoError(t, err)

	pbState1, err := state_native.ProtobufBeaconStateZond(state1.ToProto())
	require.NoError(t, err)
	pbstate, err := state_native.ProtobufBeaconStateZond(state.ToProto())
	require.NoError(t, err)

	root1, err1 := hash.Proto(pbState1)
	root2, err2 := hash.Proto(pbstate)

	if err1 != nil || err2 != nil {
		t.Fatalf("Failed to marshal state to bytes: %v %v", err1, err2)
	}
	require.DeepEqual(t, root1, root2, "Tree hash of two genesis states should be equal, received %#x == %#x", root1, root2)
}

func TestOptimizedGenesisBeaconState_RejectsActiveValidatorOverflow(t *testing.T) {
	maxActiveValidators, err := params.BeaconConfig().MaxActiveValidators()
	require.NoError(t, err)

	validators := make([]*qrysmpb.Validator, maxActiveValidators+1)
	for i := range validators {
		validators[i] = &qrysmpb.Validator{
			ActivationEpoch: params.BeaconConfig().GenesisEpoch,
			ExitEpoch:       params.BeaconConfig().FarFutureEpoch,
		}
	}
	preState, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
		Validators: validators,
	})
	require.NoError(t, err)

	_, err = transition.OptimizedGenesisBeaconStateZond(
		0,
		preState,
		&qrysmpb.ExecutionData{},
		&enginev1.ExecutionPayloadZond{},
	)
	require.ErrorContains(t, "genesis active validator count", err)
}

func TestGenesisState_InitializesLatestBlockHashes(t *testing.T) {
	deposits, _, err := util.DeterministicDepositsAndKeys(100)
	require.NoError(t, err)
	s, err := transition.GenesisBeaconStateZond(context.Background(), deposits, 0, &qrysmpb.ExecutionData{}, &enginev1.ExecutionPayloadZond{})
	require.NoError(t, err)
	got, want := uint64(len(s.BlockRoots())), uint64(params.BeaconConfig().SlotsPerHistoricalRoot)
	assert.Equal(t, want, got, "Wrong number of recent block hashes")

	got = uint64(cap(s.BlockRoots()))
	assert.Equal(t, want, got, "The slice underlying array capacity is wrong")

	for _, h := range s.BlockRoots() {
		assert.DeepEqual(t, params.BeaconConfig().ZeroHash[:], h, "Unexpected non-zero hash data")
	}
}

func TestGenesisState_FailsWithoutExecutionData(t *testing.T) {
	_, err := transition.GenesisBeaconStateZond(context.Background(), nil, 0, nil, &enginev1.ExecutionPayloadZond{})
	assert.ErrorContains(t, "no executionData provided for genesis state", err)
}

func TestGenesisBeaconState_ExecutionData(t *testing.T) {
	runGenesisStorageModes(t, func(t *testing.T) {
		deposits, _, err := util.DeterministicDepositsAndKeys(1)
		require.NoError(t, err)
		depositData, err := util.DeterministicExecutionData(len(deposits))
		require.NoError(t, err)
		emptyTrie, err := trie.NewTrie(params.BeaconConfig().DepositContractTreeDepth)
		require.NoError(t, err)
		emptyRoot, err := emptyTrie.HashTreeRoot()
		require.NoError(t, err)

		for _, tc := range []struct {
			name         string
			root         []byte
			count        uint64
			wantRoot     []byte
			invalidProof bool
			wantErr      string
		}{
			{name: "supplied premine root", root: emptyRoot[:], wantRoot: emptyRoot[:]},
			{name: "supplied deposit root", root: depositData.DepositRoot, count: 1, wantRoot: depositData.DepositRoot},
			{name: "derive nil root", count: 1, wantRoot: depositData.DepositRoot},
			{name: "derive empty root with zero count", root: []byte{}, wantRoot: depositData.DepositRoot},
			{name: "invalid proof", root: emptyRoot[:], invalidProof: true, wantErr: "deposit root did not verify"},
			{name: "invalid root length", root: []byte{1, 2, 3}, wantErr: "invalid execution deposit root length"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				executionData := &qrysmpb.ExecutionData{
					DepositRoot: tc.root, DepositCount: tc.count, BlockHash: make([]byte, 32),
				}
				before := qrysmpb.CopyExecutionData(executionData)
				deposit := proto.Clone(deposits[0]).(*qrysmpb.Deposit)
				if tc.invalidProof {
					deposit.Proof[0][0] ^= 1
				}
				st, err := transition.GenesisBeaconStateZond(context.Background(), []*qrysmpb.Deposit{deposit}, 1234, executionData, genesisExecutionPayload())
				assert.DeepEqual(t, before, executionData, "must not mutate caller execution data")
				if tc.wantErr != "" {
					require.ErrorContains(t, tc.wantErr, err)
					return
				}
				require.NoError(t, err)
				want := qrysmpb.CopyExecutionData(before)
				want.DepositRoot = tc.wantRoot
				require.DeepEqual(t, want, st.ExecutionData())
				require.Equal(t, tc.count, st.ExecutionDepositIndex())
				require.Equal(t, 1, st.NumValidators())
				nativeRoot, err := st.HashTreeRoot(context.Background())
				require.NoError(t, err)
				generatedRoot, err := st.ToProto().(*qrysmpb.BeaconStateZond).HashTreeRoot()
				require.NoError(t, err)
				require.Equal(t, nativeRoot, generatedRoot)
			})
		}
	})
}

func TestGenesisBeaconState_IndependentOfCommitteeCache(t *testing.T) {
	runGenesisStorageModes(t, func(t *testing.T) {
		ctx := context.Background()
		counts := []uint64{128, 256}
		deposits := make([][]*qrysmpb.Deposit, len(counts))
		states := make([]state.BeaconState, len(counts))
		roots := make([][32]byte, len(counts))
		build := func(t *testing.T, i int) state.BeaconState {
			t.Helper()
			st, err := transition.GenesisBeaconStateZond(ctx, deposits[i], 1234,
				&qrysmpb.ExecutionData{DepositCount: counts[i], BlockHash: make([]byte, 32)}, genesisExecutionPayload())
			require.NoError(t, err)
			return st
		}
		for i, count := range counts {
			var err error
			deposits[i], _, err = util.DeterministicDepositsAndKeys(count)
			require.NoError(t, err)
			helpers.ClearCache()
			states[i] = build(t, i)
			roots[i], err = states[i].HashTreeRoot(ctx)
			require.NoError(t, err)
		}
		for _, order := range [][2]int{{0, 1}, {1, 0}} {
			cached, target := order[0], order[1]
			t.Run(fmt.Sprintf("%d_then_%d", counts[cached], counts[target]), func(t *testing.T) {
				helpers.ClearCache()
				// Explicitly populate a conflicting cache entry, even if genesis
				// construction itself no longer writes to the shared cache.
				require.NoError(t, helpers.UpdateCommitteeCache(ctx, states[cached], 1))
				st := build(t, target)
				root, err := st.HashTreeRoot(ctx)
				require.NoError(t, err)
				require.Equal(t, roots[target], root, "genesis root must not depend on cached registry")
				wantCommittee, err := states[target].CurrentSyncCommittee()
				require.NoError(t, err)
				current, err := st.CurrentSyncCommittee()
				require.NoError(t, err)
				next, err := st.NextSyncCommittee()
				require.NoError(t, err)
				require.DeepEqual(t, wantCommittee, current)
				require.DeepEqual(t, wantCommittee, next)
			})
		}
	})
}

func runGenesisStorageModes(t *testing.T, test func(*testing.T)) {
	t.Helper()
	for _, experimental := range []bool{false, true} {
		t.Run(fmt.Sprintf("experimental=%t", experimental), func(t *testing.T) {
			flags := *features.Get()
			flags.EnableExperimentalState = experimental
			t.Cleanup(features.InitWithReset(&flags))
			helpers.ClearCache()
			t.Cleanup(helpers.ClearCache)
			test(t)
		})
	}
}

func genesisExecutionPayload() *enginev1.ExecutionPayloadZond {
	return &enginev1.ExecutionPayloadZond{
		ParentHash:    make([]byte, 32),
		FeeRecipient:  make([]byte, fieldparams.FeeRecipientLength),
		StateRoot:     make([]byte, 32),
		ReceiptsRoot:  make([]byte, 32),
		LogsBloom:     make([]byte, 256),
		PrevRandao:    make([]byte, 32),
		BaseFeePerGas: make([]byte, 32),
		BlockHash:     make([]byte, 32),
	}
}
