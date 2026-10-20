package blockchain

import (
	"context"
	"encoding/json"
	"testing"
	"testing/synctest"
	"time"

	elengine "github.com/theQRL/go-qrl/beacon/engine"
	"github.com/theQRL/go-qrl/common"
	coreblocks "github.com/theQRL/qrysm/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	"github.com/theQRL/qrysm/beacon-chain/execution"
	mockExecution "github.com/theQRL/qrysm/beacon-chain/execution/testing"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

type delayedNewPayloadEngine struct {
	*mockExecution.EngineClient
	delay time.Duration
	calls int
}

func (e *delayedNewPayloadEngine) NewPayload(ctx context.Context, payload interfaces.ExecutionData, hashes []common.Hash, root *common.Hash) ([]byte, error) {
	e.calls++
	time.Sleep(e.delay)
	return e.EngineClient.NewPayload(ctx, payload, hashes, root)
}

func TestService_ReceiveBlock_ConsensusBeforeInvalidation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		batch bool
		delay time.Duration
	}{
		{name: "gossip immediate execution response"},
		{name: "gossip delayed execution response", delay: time.Second},
		{name: "batch", batch: true},
	} {
		for _, badParent := range []bool{false, true} {
			name := tc.name + map[bool]string{false: "/consensus valid", true: "/wrong execution parent"}[badParent]
			t.Run(name, func(t *testing.T) {
				f := newBatchExecutionFixture(t, 3)
				block := f.blks[2]
				if badParent {
					block = blockWithWrongExecutionParent(t, f)
				}
				parentPayload, err := f.blks[0].Block().Body().Execution()
				require.NoError(t, err)
				f.engine.ErrNewPayload = execution.ErrInvalidPayloadStatus
				f.engine.NewPayloadResp = bytesutil.SafeCopyBytes(parentPayload.BlockHash())
				engine := &delayedNewPayloadEngine{EngineClient: f.engine, delay: tc.delay}
				f.s.cfg.ExecutionEngineCaller = engine
				synctest.Test(t, func(t *testing.T) {
					t.Cleanup(synctest.Wait)
					driftGenesisTime(f.s, 20, 0)
					ancestor := f.blks[1].Root()
					require.Equal(t, true, f.s.cfg.ForkChoiceStore.HasNode(ancestor))
					require.Equal(t, true, f.s.cfg.BeaconDB.HasBlock(f.ctx, ancestor))
					require.Equal(t, true, f.s.cfg.BeaconDB.HasStateSummary(f.ctx, ancestor))
					if tc.batch {
						err = f.s.ReceiveBlockBatch(f.ctx, []blocks.ROBlock{block})
					} else {
						err = f.s.ReceiveBlock(f.ctx, block, block.Root())
					}
					synctest.Wait()
					require.Equal(t, true, IsInvalidBlock(err))
					if badParent {
						require.ErrorIs(t, err, coreblocks.ErrInvalidPayloadBlockHash)
						assert.Equal(t, 0, len(InvalidAncestorRoots(err)), "consensus failure must not blame beacon ancestors")
						assert.Equal(t, [32]byte{}, InvalidBlockLVH(err))
						assert.Equal(t, ancestor, f.s.CachedHeadRoot())
					} else {
						require.ErrorContains(t, "received an INVALID payload", err)
						assert.Equal(t, true, len(InvalidAncestorRoots(err)) > 0, "consensus-valid blocks must still invalidate execution ancestors")
					}
					wantCalls := 1
					if tc.batch && badParent {
						wantCalls = 0
					}
					assert.Equal(t, wantCalls, engine.calls)
					assert.Equal(t, badParent, f.s.cfg.ForkChoiceStore.HasNode(ancestor))
					assert.Equal(t, badParent, f.s.cfg.BeaconDB.HasBlock(f.ctx, ancestor))
					assert.Equal(t, badParent, f.s.cfg.BeaconDB.HasStateSummary(f.ctx, ancestor))
					assert.Equal(t, false, f.s.cfg.ForkChoiceStore.HasNode(block.Root()))
					assert.Equal(t, false, f.s.cfg.BeaconDB.HasBlock(f.ctx, block.Root()))
				})
			})
		}
	}
}

// blockWithWrongExecutionParent passes gossip checks but has different beacon
// and execution ancestors. A is execution-valid, B is its optimistic beacon
// child, and C names B as its beacon parent but A as its execution parent.
func blockWithWrongExecutionParent(t *testing.T, f *batchExecutionFixture) blocks.ROBlock {
	t.Helper()
	copied, err := f.blks[2].Copy()
	require.NoError(t, err)
	pb, err := copied.PbZondBlock()
	require.NoError(t, err)
	parentPayload, err := f.blks[0].Block().Body().Execution()
	require.NoError(t, err)
	payload := pb.Block.Body.ExecutionPayload
	payload.ParentHash = bytesutil.SafeCopyBytes(parentPayload.BlockHash())
	payload.BlockNumber = parentPayload.BlockNumber() + 1
	// The EL rejects this gas usage and reports A as latestValidHash. B is
	// not an ancestor of C's execution payload and must not be invalidated.
	payload.GasUsed = payload.GasLimit + 1
	encoded, err := json.Marshal(payload)
	require.NoError(t, err)
	var data elengine.ExecutableData
	require.NoError(t, json.Unmarshal(encoded, &data))
	executionBlock, err := elengine.ExecutableDataToBlockNoHash(data)
	require.NoError(t, err)
	payload.BlockHash = executionBlock.Hash().Bytes()
	data.BlockHash = executionBlock.Hash()
	_, err = elengine.ExecutableDataToBlock(data)
	require.NoError(t, err, "execution block hash must be correct")
	domain, err := signing.Domain(f.states[2].Fork(), 0, params.BeaconConfig().DomainBeaconProposer, f.states[2].GenesisValidatorsRoot())
	require.NoError(t, err)
	root, err := signing.ComputeSigningRoot(pb.Block, domain)
	require.NoError(t, err)
	sig, err := f.keys[pb.Block.ProposerIndex].Sign(root[:])
	require.NoError(t, err)
	pb.Signature = sig.Marshal()
	signed, err := blocks.NewSignedBeaconBlock(pb)
	require.NoError(t, err)
	block, err := blocks.NewROBlock(signed)
	require.NoError(t, err)
	require.NoError(t, coreblocks.VerifyBlockSignature(f.states[2], block.Block().ProposerIndex(), pb.Signature, block.Block().HashTreeRoot))
	require.NoError(t, coreblocks.VerifyBlockSignatureUsingCurrentFork(f.states[2], block, block.Root()))
	// Verify the remaining gossip proposer and timestamp checks. Gossip does
	// not compare the payload's execution parent to its beacon parent state.
	gossipState, err := transition.ProcessSlots(f.ctx, f.states[2].Copy(), block.Block().Slot())
	require.NoError(t, err)
	proposer, err := helpers.BeaconProposerIndex(f.ctx, gossipState)
	require.NoError(t, err)
	require.Equal(t, proposer, block.Block().ProposerIndex())
	require.Equal(t, gossipState.GenesisTime()+uint64(block.Block().Slot())*params.BeaconConfig().SecondsPerSlot, payload.Timestamp)
	return block
}
