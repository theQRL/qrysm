package blockchain

import (
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	mockExecution "github.com/theQRL/qrysm/beacon-chain/execution/testing"
	"github.com/theQRL/qrysm/beacon-chain/operations/voluntaryexits"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	"github.com/theQRL/qrysm/time/slots"
	"google.golang.org/protobuf/proto"
)

// signWithdrawalProbeBlock signs the (already mutated) block with the proposer's
// key. The state root is intentionally NOT recomputed: with the fix a block
// carrying an invalid withdrawal has no valid post-state, so ProcessWithdrawals
// rejects it during the transition before the state-root check is reached.
func signWithdrawalProbeBlock(t *testing.T, st state.BeaconState, keys []ml_dsa_87.MLDSA87Key, b *qrysmpb.SignedBeaconBlockZond) {
	t.Helper()
	d, err := signing.Domain(st.Fork(), slots.ToEpoch(b.Block.Slot), params.BeaconConfig().DomainBeaconProposer, st.GenesisValidatorsRoot())
	require.NoError(t, err)
	r, err := signing.ComputeSigningRoot(b.Block, d)
	require.NoError(t, err)
	idx := uint64(b.Block.ProposerIndex) % uint64(len(keys))
	sig, err := keys[idx].Sign(r[:])
	require.NoError(t, err)
	b.Signature = sig.Marshal()
}

// TestFix_WithdrawalBearingEmptyPayloadRejected verifies the IsEmptyExecutionData
// withdrawals fix end to end: an otherwise all-zero execution payload that carries
// a single withdrawal must (1) be detected as non-empty, and (2) be rejected before
// storage on both the single-block and batch acceptance paths, with nothing left in
// forkchoice, the database, or stategen.
func TestFix_WithdrawalBearingEmptyPayloadRejected(t *testing.T) {
	genesis, keys := util.DeterministicGenesisStateZond(t, 128)
	base, err := util.GenerateFullBlockZond(genesis.Copy(), keys, nil, 1)
	require.NoError(t, err)
	// An all-zero payload except for one fabricated withdrawal. The genesis kept
	// its default empty execution header, so execution is not yet enabled: exactly
	// the condition under which the bug misclassified this payload as empty.
	base.Block.Body.ExecutionPayload = &enginev1.ExecutionPayloadZond{
		ParentHash:    make([]byte, 32),
		FeeRecipient:  make([]byte, 64),
		StateRoot:     make([]byte, 32),
		ReceiptsRoot:  make([]byte, 32),
		LogsBloom:     make([]byte, 256),
		PrevRandao:    make([]byte, 32),
		BaseFeePerGas: make([]byte, 32),
		BlockHash:     make([]byte, 32),
		Withdrawals:   []*enginev1.Withdrawal{{Index: 999, ValidatorIndex: 0, Address: make([]byte, 64), Amount: 1}},
	}

	// (1) Detection: the payload is no longer classified as empty.
	probe, err := blocks.NewSignedBeaconBlock(base)
	require.NoError(t, err)
	payload, err := probe.Block().Body().Execution()
	require.NoError(t, err)
	empty, err := blocks.IsEmptyExecutionData(payload)
	require.NoError(t, err)
	require.Equal(t, false, empty)

	// (2) Rejection before storage on both acceptance paths.
	for _, path := range []string{"single", "batch"} {
		t.Run(path, func(t *testing.T) {
			b := proto.Clone(base).(*qrysmpb.SignedBeaconBlockZond)
			signWithdrawalProbeBlock(t, genesis, keys, b)

			// A VALID engine so the only rejection source is consensus processing.
			eng := &mockExecution.EngineClient{}
			s, tr := minimalTestService(t, WithExecutionEngineCaller(eng), WithExitPool(voluntaryexits.NewPool()))
			require.NoError(t, s.saveGenesisData(tr.ctx, genesis.Copy()))

			w, err := blocks.NewSignedBeaconBlock(b)
			require.NoError(t, err)
			ro, err := blocks.NewROBlock(w)
			require.NoError(t, err)

			if path == "single" {
				err = s.ReceiveBlock(tr.ctx, w, ro.Root())
			} else {
				err = s.ReceiveBlockBatch(tr.ctx, []blocks.ROBlock{ro})
			}
			if err == nil {
				t.Fatalf("INVALID BLOCK ACCEPTED via %s; saved=%v forkchoice=%v", path, tr.db.HasBlock(tr.ctx, ro.Root()), s.InForkchoice(ro.Root()))
			}
			t.Logf("%s rejected: %v", path, err)
			require.ErrorContains(t, "withdrawals", err)
			require.Equal(t, false, s.InForkchoice(ro.Root()))
			require.Equal(t, false, tr.db.HasBlock(tr.ctx, ro.Root()))
			has, e := tr.sg.HasState(tr.ctx, ro.Root())
			require.NoError(t, e)
			require.Equal(t, false, has)
		})
	}
}
