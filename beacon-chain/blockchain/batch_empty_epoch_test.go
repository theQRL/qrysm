package blockchain

import (
	"testing"
	"testing/synctest"

	"github.com/theQRL/qrysm/beacon-chain/core/epoch/precompute"
	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestReceiveBlockBatch_EmptyEpoch(t *testing.T) {
	setupEpochTransitionTest(t)
	for _, cacheCheckpoint := range []bool{true, false} {
		t.Run(map[bool]string{true: "checkpoint cached control", false: "normal batch"}[cacheCheckpoint], func(t *testing.T) {
			f := newBatchExecutionFixture(t, 11)
			f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
			pre := f.states[11]
			pb, err := util.GenerateFullBlockZond(pre.Copy(), f.keys, &util.BlockGenConfig{}, 18)
			require.NoError(t, err)
			// No blocks in epoch 2, but validators still attest to A11.
			// The first block of epoch 3 includes those valid votes.
			// These four committees contain 44 of 64 validators, enough for
			// justification while respecting the four-attestation block limit.
			for _, slot := range []primitives.Slot{13, 14, 16, 17} {
				atSlot, err := transition.ProcessSlots(f.ctx, pre.Copy(), slot)
				require.NoError(t, err)
				atts, err := util.GenerateAttestations(atSlot, f.keys, 1, slot, false)
				require.NoError(t, err)
				pb.Block.Body.Attestations = append(pb.Block.Body.Attestations, atts...)
			}
			sig, err := util.BlockSignature(pre.Copy(), pb.Block, f.keys)
			require.NoError(t, err)
			pb.Signature = sig.Marshal()
			signed, err := blocks.NewSignedBeaconBlock(pb)
			require.NoError(t, err)
			last, err := blocks.NewROBlock(signed)
			require.NoError(t, err)
			post, err := transition.ExecuteStateTransition(f.ctx, pre.Copy(), last)
			require.NoError(t, err, "full signature and transition validation")
			uj, _, err := precompute.UnrealizedCheckpoints(post)
			require.NoError(t, err)
			require.Equal(t, primitives.Epoch(2), uj.Epoch)
			branch := append(append([]blocks.ROBlock(nil), f.blks[2:]...), last)
			if cacheCheckpoint {
				require.NoError(t, f.s.cfg.StateGen.SaveState(f.ctx, f.blks[10].Root(), pre.Copy()))
			}
			synctest.Test(t, func(t *testing.T) {
				t.Cleanup(synctest.Wait)
				driftGenesisTime(f.s, 24, -30)
				require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, branch))
				synctest.Wait()
				published, err := f.s.HeadRoot(f.ctx)
				require.NoError(t, err)
				require.Equal(t, last.Root(), bytesutil.ToBytes32(published))
				jc := f.s.cfg.ForkChoiceStore.JustifiedCheckpoint()
				require.Equal(t, primitives.Epoch(2), jc.Epoch)
				require.Equal(t, f.blks[10].Root(), jc.Root)
			})
		})
	}
}
