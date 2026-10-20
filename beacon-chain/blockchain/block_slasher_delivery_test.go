package blockchain

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"

	"github.com/theQRL/qrysm/async/event"
	"github.com/theQRL/qrysm/beacon-chain/execution"
	"github.com/theQRL/qrysm/config/features"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestReceiveBlock_RetainedSlasherAttestations(t *testing.T) {
	setupEpochTransitionTest(t)
	for _, mode := range []string{
		"healthy", "cancel after insertion", "head write failure", "checkpoint write failure",
		"invalid signature", "invalid payload", "invalid forkchoice head", "invalid competing head",
	} {
		t.Run(mode, func(t *testing.T) {
			count := 3
			if mode == "checkpoint write failure" {
				count = 24
			}
			f := newBatchExecutionFixture(t, count)
			f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
			flags := *features.Get()
			flags.EnableSlasher = true
			t.Cleanup(features.InitWithReset(&flags))
			f.s.cfg.SlasherAttestationsFeed = &event.Feed{}
			b := f.blks[count-1]
			if mode == "invalid competing head" {
				pb, err := util.GenerateFullBlockZond(f.states[1].Copy(), f.keys, util.DefaultBlockGenConfig(), 3)
				require.NoError(t, err)
				signed, err := blocks.NewSignedBeaconBlock(pb)
				require.NoError(t, err)
				b, err = blocks.NewROBlock(signed)
				require.NoError(t, err)
			} else if mode == "invalid signature" {
				pb, err := b.PbZondBlock()
				require.NoError(t, err)
				pb.Signature[0] ^= 1
				signed, err := blocks.NewSignedBeaconBlock(pb)
				require.NoError(t, err)
				b, err = blocks.NewROBlock(signed)
				require.NoError(t, err)
			}
			want := len(b.Block().Body().Attestations())
			require.Equal(t, true, want > 0)
			synctest.Test(t, func(t *testing.T) {
				t.Cleanup(synctest.Wait)
				slot := primitives.Slot(4)
				if count == 24 {
					driftGenesisTime(f.s, 23, 0)
					require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[2:23]))
					synctest.Wait()
					slot = 24
				}
				driftGenesisTime(f.s, int64(slot), 0)
				request, cancel := context.WithCancel(f.ctx)
				defer cancel()
				fc := f.s.cfg.ForkChoiceStore
				d := &finalityHoldHeadRootDB{HeadAccessDatabase: f.s.cfg.BeaconDB, hold: mode == "head write failure"}
				f.s.cfg.BeaconDB = d
				writeErr := errors.New("temporary finalized checkpoint write failure")
				switch mode {
				case "cancel after insertion":
					f.s.cfg.ForkChoiceStore = &notificationInterruptedStore{ForkChoicer: fc, cancel: cancel}
				case "checkpoint write failure":
					f.s.cfg.BeaconDB = &finalizationRetryDB{
						HeadAccessDatabase: d, operation: "finality write", failures: 1, failure: writeErr,
					}
				case "invalid payload":
					f.engine.ErrNewPayload = execution.ErrInvalidPayloadStatus
				case "invalid forkchoice head", "invalid competing head":
					validPayload, err := f.blks[0].Block().Body().Execution()
					require.NoError(t, err)
					f.engine.ErrNewPayload = execution.ErrAcceptedSyncingPayloadStatus
					f.engine.ErrForkchoiceUpdated = execution.ErrInvalidPayloadStatus
					f.engine.ForkChoiceUpdatedResp = validPayload.BlockHash()
					f.engine.OverrideValidHash = bytesutil.ToBytes32(validPayload.BlockHash())
					if mode == "invalid competing head" {
						voteForRoot(t, f, f.blks[1].Root(), 0)
						payload, err := b.Block().Body().Execution()
						require.NoError(t, err)
						f.engine.OverrideValidHash = bytesutil.ToBytes32(payload.BlockHash())
					}
				}
				attestations := make(chan *qrysmpb.IndexedAttestation, 128)
				sub := f.s.cfg.SlasherAttestationsFeed.Subscribe(attestations)
				defer sub.Unsubscribe()
				err := f.s.ReceiveBlock(request, b, b.Root())
				rejected := false
				switch mode {
				case "healthy":
					require.NoError(t, err)
				case "cancel after insertion":
					require.ErrorIs(t, err, context.Canceled)
				case "head write failure":
					require.ErrorContains(t, "temporary head persistence failure", err)
				case "checkpoint write failure":
					require.ErrorIs(t, err, writeErr)
				case "invalid competing head":
					require.Equal(t, true, IsUnrelatedBlockError(err))
					require.Equal(t, f.blks[1].Root(), InvalidBlockRoot(err))
				default:
					require.Equal(t, true, IsInvalidBlock(err))
					rejected = true
				}
				synctest.Wait()
				f.s.cfg.ForkChoiceStore = fc
				require.Equal(t, !rejected, fc.HasNode(b.Root()))
				if rejected {
					assert.Equal(t, 0, len(attestations), "rejected imports must not reach the slasher")
					return
				}
				d.hold = false
				require.NoError(t, f.s.ReceiveBlock(f.ctx, b, b.Root()))
				require.NoError(t, f.s.NewSlot(f.ctx, slot))
				f.s.UpdateHead(f.ctx, slot)
				synctest.Wait()
				require.Equal(t, b.Root(), f.s.CachedHeadRoot())
				require.Equal(t, want, len(attestations), "retained block attestations must reach the slasher exactly once")
				for _, att := range b.Block().Body().Attestations() {
					indexed := <-attestations
					assert.DeepEqual(t, att.Data, indexed.Data)
					assert.Equal(t, len(att.Signatures), len(indexed.Signatures))
				}
			})
		})
	}
}
