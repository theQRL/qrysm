package blockchain

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/theQRL/go-qrl/common"
	"github.com/theQRL/qrysm/beacon-chain/db"
	"github.com/theQRL/qrysm/beacon-chain/execution"
	mockExecution "github.com/theQRL/qrysm/beacon-chain/execution/testing"
	"github.com/theQRL/qrysm/beacon-chain/forkchoice"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	payloadattribute "github.com/theQRL/qrysm/consensus-types/payload-attribute"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

type interruptImportLock struct {
	forkchoice.ForkChoicer
	interrupt func()
}

func (f *interruptImportLock) Lock() {
	if f.interrupt != nil {
		interrupt := f.interrupt
		f.interrupt = nil
		interrupt()
	}
	f.ForkChoicer.Lock()
}

type interruptInvalidResponse struct {
	*mockExecution.EngineClient
	interrupt func()
}

func (e *interruptInvalidResponse) NewPayload(ctx context.Context, p interfaces.ExecutionData, hashes []common.Hash, root *common.Hash) ([]byte, error) {
	lvh, err := e.EngineClient.NewPayload(ctx, p, hashes, root)
	if errors.Is(err, execution.ErrInvalidPayloadStatus) {
		e.interrupt()
	}
	return lvh, err
}

func (e *interruptInvalidResponse) ForkchoiceUpdated(ctx context.Context, fcs *enginev1.ForkchoiceState, attr payloadattribute.Attributer) (*enginev1.PayloadIDBytes, []byte, error) {
	pid, lvh, err := e.EngineClient.ForkchoiceUpdated(ctx, fcs, attr)
	if errors.Is(err, execution.ErrInvalidPayloadStatus) {
		e.interrupt()
	}
	return pid, lvh, err
}

func TestService_InvalidationSurvivesImportCancellation(t *testing.T) {
	for _, mode := range []string{"gossip", "batch NewPayload", "batch ForkchoiceUpdated"} {
		for _, deadline := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/deadline=%v", mode, deadline), func(t *testing.T) {
				f := newBatchExecutionFixture(t, 3)
				validPayload, err := f.blks[0].Block().Body().Execution()
				require.NoError(t, err)
				synctest.Test(t, func(t *testing.T) {
					t.Cleanup(synctest.Wait)
					driftGenesisTime(f.s, 5, 0)
					ctx, cancel := context.WithCancel(f.ctx)
					wantErr := context.Canceled
					if deadline {
						cancel()
						ctx, cancel = context.WithTimeout(f.ctx, time.Second)
						wantErr = context.DeadlineExceeded
					}
					defer cancel()
					interrupt := func() { cancel() }
					if deadline {
						interrupt = func() { time.Sleep(2 * time.Second) }
					}
					fc := f.s.cfg.ForkChoiceStore
					if mode == "batch ForkchoiceUpdated" {
						f.engine.ErrForkchoiceUpdated = execution.ErrInvalidPayloadStatus
						f.engine.ForkChoiceUpdatedResp = validPayload.BlockHash()
					} else {
						f.engine.ErrNewPayload = execution.ErrInvalidPayloadStatus
						f.engine.NewPayloadResp = validPayload.BlockHash()
					}
					if mode == "gossip" {
						// Consensus and the INVALID response have completed before
						// cancellation races acquisition of the import's write lock.
						f.s.cfg.ForkChoiceStore = &interruptImportLock{ForkChoicer: fc, interrupt: interrupt}
						err = f.s.ReceiveBlock(ctx, f.blks[2], f.blks[2].Root())
						f.s.cfg.ForkChoiceStore = fc
					} else {
						// Batches already hold the lock. Cancel as the engine's
						// rejection arrives, before applying its invalidation.
						f.s.cfg.ExecutionEngineCaller = &interruptInvalidResponse{EngineClient: f.engine, interrupt: interrupt}
						err = f.s.ReceiveBlockBatch(ctx, f.blks[2:])
						f.s.cfg.ExecutionEngineCaller = f.engine
					}
					require.Equal(t, true, IsInvalidBlock(err))
					require.ErrorIs(t, err, wantErr)
					require.Equal(t, f.blks[2].Root(), InvalidBlockRoot(err))
					require.Equal(t, bytesutil.ToBytes32(validPayload.BlockHash()), InvalidBlockLVH(err))
					root := f.blks[1].Root()
					found := false
					for _, invalidRoot := range InvalidAncestorRoots(err) {
						found = found || invalidRoot == root
					}
					require.Equal(t, true, found, "retain invalid ancestors in the error")
					require.Equal(t, false, f.s.InForkchoice(root), "cancellation must not preserve a known-invalid branch")
					require.Equal(t, false, f.s.HasBlock(f.ctx, root), "quarantine the block before cancelable storage work")
					require.Equal(t, true, f.s.cfg.BeaconDB.HasBlock(f.ctx, root), "storage cleanup remains retryable")
					_, pending := f.s.pendingInvalidBlocks[root]
					require.Equal(t, true, pending)
					f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
					f.s.UpdateHead(f.ctx, 5)
					synctest.Wait()
					require.Equal(t, false, f.s.cfg.BeaconDB.HasBlock(f.ctx, root))
					require.Equal(t, 0, len(f.s.pendingInvalidBlocks))
					require.Equal(t, 0, len(f.s.invalidatedHeadBlocks))
					published, err := f.s.HeadRoot(f.ctx)
					require.NoError(t, err)
					require.Equal(t, f.blks[0].Root(), bytesutil.ToBytes32(published))
				})
			})
		}
	}
}

type invalidCleanupRetryDB struct {
	db.HeadAccessDatabase
	root      [32]byte
	err       error
	remaining int
	attempts  int
}

func TestService_InvalidBlockCleanupInterruptedRecovery(t *testing.T) {
	f := newBatchExecutionFixture(t, 4)
	payload, err := f.blks[0].Block().Body().Execution()
	require.NoError(t, err)
	synctest.Test(t, func(t *testing.T) {
		t.Cleanup(synctest.Wait)
		driftGenesisTime(f.s, 5, 0)
		require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[2:3]))
		synctest.Wait()
		// Model an invalid ancestor whose only block copy is in the initial
		// sync cache. Recovery still needs its reusable vote for the valid A1.
		root := f.blks[1].Root()
		require.Equal(t, true, f.s.hasInitSyncBlock(root))
		require.NoError(t, f.s.cfg.BeaconDB.DeleteBlock(f.ctx, root))
		f.s.cfg.ForkChoiceStore.Lock()
		roots, err := f.s.cfg.ForkChoiceStore.SetOptimisticToInvalid(f.ctx,
			f.blks[3].Root(), f.blks[2].Root(), bytesutil.ToBytes32(payload.BlockHash()))
		require.NoError(t, err)
		canceled, cancel := context.WithCancel(f.ctx)
		cancel()
		err = f.s.removeInvalidBlockAndState(canceled, roots)
		f.s.cfg.ForkChoiceStore.Unlock()
		require.ErrorIs(t, err, context.Canceled)
		require.Equal(t, false, f.s.hasInitSyncBlock(root))
		require.Equal(t, false, f.s.HasBlock(f.ctx, root))
		_, err = f.s.getBlock(f.ctx, root)
		require.Equal(t, true, IsInvalidBlock(err))
		f.engine.ErrForkchoiceUpdated = nil
		f.s.UpdateHead(f.ctx, 5)
		synctest.Wait()
		published, err := f.s.HeadRoot(f.ctx)
		require.NoError(t, err)
		require.Equal(t, f.blks[0].Root(), bytesutil.ToBytes32(published))
		atts, err := f.s.cfg.AttPool.UnaggregatedAttestations()
		require.NoError(t, err)
		atts = append(atts, f.s.cfg.AttPool.AggregatedAttestations()...)
		require.Equal(t, 1, len(atts))
		require.DeepEqual(t, f.blks[1].Block().Body().Attestations()[0], atts[0])
		require.Equal(t, 0, len(f.s.pendingInvalidBlocks))
		require.Equal(t, 0, len(f.s.invalidatedHeadBlocks))
		require.NoError(t, f.s.saveInitSyncBlocks(f.ctx, false))
		require.Equal(t, false, f.s.cfg.BeaconDB.HasBlock(f.ctx, root))
	})
}

type inflightInvalidationEngine struct {
	*mockExecution.EngineClient
	delayed, invalid, lastValid [32]byte
	entered, release            chan struct{}
	waiting                     bool
}

func (e *inflightInvalidationEngine) NewPayload(_ context.Context, payload interfaces.ExecutionData, _ []common.Hash, _ *common.Hash) ([]byte, error) {
	hash := bytesutil.ToBytes32(payload.BlockHash())
	if hash == e.delayed && !e.waiting {
		e.waiting = true
		close(e.entered)
		<-e.release
	}
	if hash == e.invalid {
		return e.lastValid[:], execution.ErrInvalidPayloadStatus
	}
	return nil, execution.ErrAcceptedSyncingPayloadStatus
}

func TestService_InvalidBlockCleanupInFlight(t *testing.T) {
	f := newBatchExecutionFixture(t, 4)
	var hashes [][32]byte
	for _, b := range f.blks[1:] {
		payload, err := b.Block().Body().Execution()
		require.NoError(t, err)
		hashes = append(hashes, bytesutil.ToBytes32(payload.BlockHash()))
	}
	e := &inflightInvalidationEngine{
		EngineClient: f.engine, delayed: hashes[1], invalid: hashes[2], lastValid: hashes[0],
	}
	f.s.cfg.ExecutionEngineCaller = e
	synctest.Test(t, func(t *testing.T) {
		t.Cleanup(synctest.Wait)
		e.entered, e.release = make(chan struct{}), make(chan struct{})
		release := sync.OnceFunc(func() { close(e.release) })
		defer release()
		driftGenesisTime(f.s, 5, 0)
		result := make(chan error, 1)
		go func() { result <- f.s.ReceiveBlock(f.ctx, f.blks[2], f.blks[2].Root()) }()
		<-e.entered
		synctest.Wait()
		require.Equal(t, false, f.s.HasBlock(f.ctx, f.blks[2].Root()), "the pending import is not available yet")
		// The batch imports C3 while gossip is waiting on the same payload.
		// D4 then invalidates C3, leaving its parent B2 available for imports.
		require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[2:3]))
		synctest.Wait()
		require.Equal(t, true, f.s.BlockBeingSynced(f.blks[2].Root()))
		require.Equal(t, true, f.s.HasBlock(f.ctx, f.blks[2].Root()), "a completed batch import remains available while duplicate gossip waits")
		err := f.s.ReceiveBlock(f.ctx, f.blks[3], f.blks[3].Root())
		require.Equal(t, true, IsInvalidBlock(err))
		f.s.UpdateHead(f.ctx, 5)
		synctest.Wait()
		require.Equal(t, true, f.s.cfg.ForkChoiceStore.HasNode(f.blks[1].Root()))
		require.Equal(t, false, f.s.cfg.BeaconDB.HasBlock(f.ctx, f.blks[2].Root()))
		release()
		err = <-result
		require.Equal(t, true, IsInvalidBlock(err), "the earlier SYNCING response must not reinsert C3")
		require.Equal(t, f.blks[2].Root(), InvalidBlockRoot(err))
		f.s.UpdateHead(f.ctx, 5)
		synctest.Wait()
		require.Equal(t, false, f.s.cfg.ForkChoiceStore.HasNode(f.blks[2].Root()))
		require.Equal(t, false, f.s.cfg.BeaconDB.HasBlock(f.ctx, f.blks[2].Root()))
		require.Equal(t, 0, len(f.s.pendingInvalidBlocks))
		published, err := f.s.HeadRoot(f.ctx)
		require.NoError(t, err)
		require.Equal(t, f.blks[1].Root(), bytesutil.ToBytes32(published))
	})
}

func (d *invalidCleanupRetryDB) DeleteBlock(ctx context.Context, root [32]byte) error {
	if root == d.root {
		d.attempts++
		if d.remaining > 0 {
			d.remaining--
			return d.err
		}
	}
	return d.HeadAccessDatabase.DeleteBlock(ctx, root)
}

func TestService_InvalidBlockCleanupRetry(t *testing.T) {
	for _, mode := range []string{"NewPayload", "batch NewPayload", "ForkchoiceUpdated"} {
		for _, fail := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/fail=%v", mode, fail), func(t *testing.T) {
				f := newBatchExecutionFixture(t, 4)
				payload, err := f.blks[0].Block().Body().Execution()
				require.NoError(t, err)
				synctest.Test(t, func(t *testing.T) {
					t.Cleanup(synctest.Wait)
					driftGenesisTime(f.s, 5, 0)
					require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[2:3]))
					synctest.Wait()
					d := &invalidCleanupRetryDB{
						HeadAccessDatabase: f.s.cfg.BeaconDB,
						root:               f.blks[1].Root(),
						err:                errors.New("temporary block deletion failure"),
					}
					if fail {
						d.remaining = 1
					}
					f.s.cfg.BeaconDB = d
					f.engine.ErrForkchoiceUpdated = nil
					if mode == "ForkchoiceUpdated" {
						f.engine.ErrForkchoiceUpdated = execution.ErrInvalidPayloadStatus
						f.engine.ForkChoiceUpdatedResp = payload.BlockHash()
						f.engine.OverrideValidHash = bytesutil.ToBytes32(payload.BlockHash())
					} else {
						f.engine.ErrNewPayload = execution.ErrInvalidPayloadStatus
						f.engine.NewPayloadResp = payload.BlockHash()
					}
					if mode == "batch NewPayload" {
						err = f.s.ReceiveBlockBatch(f.ctx, f.blks[3:])
					} else {
						err = f.s.ReceiveBlock(f.ctx, f.blks[3], f.blks[3].Root())
					}
					require.NotNil(t, err)
					if fail {
						require.ErrorIs(t, err, d.err)
					}
					assert.Equal(t, true, IsInvalidBlock(err), "cleanup failure must retain the INVALID verdict for sync")
					assert.Equal(t, f.blks[3].Root(), InvalidBlockRoot(err))
					assert.Equal(t, bytesutil.ToBytes32(payload.BlockHash()), InvalidBlockLVH(err))
					found := false
					for _, root := range InvalidAncestorRoots(err) {
						found = found || root == d.root
					}
					assert.Equal(t, true, found, "sync must receive the invalid ancestor even when deletion fails")
					synctest.Wait()
					if fail {
						require.Equal(t, true, d.HasBlock(f.ctx, d.root))
						require.Equal(t, false, f.s.HasBlock(f.ctx, d.root), "hide the invalid block while deletion is pending")
						_, err := f.s.getBlock(f.ctx, d.root)
						require.Equal(t, true, IsInvalidBlock(err))
					}
					for slot := primitives.Slot(5); slot <= 7; slot++ {
						driftGenesisTime(f.s, int64(slot), 0)
						require.NoError(t, f.s.NewSlot(f.ctx, slot))
						f.s.UpdateHead(f.ctx, slot)
						synctest.Wait()
					}
					root, err := f.s.HeadRoot(f.ctx)
					require.NoError(t, err)
					require.Equal(t, f.blks[0].Root(), bytesutil.ToBytes32(root))
					require.Equal(t, false, f.s.cfg.ForkChoiceStore.HasNode(d.root))
					require.Equal(t, 0, len(f.s.invalidatedHeadBlocks))
					attempts := 1
					if fail {
						attempts++
					}
					require.Equal(t, attempts, d.attempts)
					require.Equal(t, 0, len(f.s.pendingInvalidBlocks))
					assert.Equal(t, false, d.HasBlock(f.ctx, d.root), "invalid block must not remain stored after retry succeeds")
					assert.Equal(t, false, f.s.HasBlock(f.ctx, d.root), "invalid block must not remain available through the service")
				})
			})
		}
	}
}

func TestService_InvalidBlockCleanupBackfill(t *testing.T) {
	for _, failures := range []int{0, 1, 100} {
		t.Run(fmt.Sprintf("deletion failures=%d", failures), func(t *testing.T) {
			f := newBatchExecutionFixture(t, 4)
			payload, err := f.blks[0].Block().Body().Execution()
			require.NoError(t, err)
			synctest.Test(t, func(t *testing.T) {
				t.Cleanup(synctest.Wait)
				driftGenesisTime(f.s, 5, 0)
				d := &invalidCleanupRetryDB{
					HeadAccessDatabase: f.s.cfg.BeaconDB,
					root:               f.blks[1].Root(), err: errors.New("temporary block deletion failure"),
					remaining: failures,
				}
				f.s.cfg.BeaconDB = d
				f.engine.ErrNewPayload = execution.ErrInvalidPayloadStatus
				f.engine.NewPayloadResp = payload.BlockHash()
				err = f.s.ReceiveBlock(f.ctx, f.blks[2], f.blks[2].Root())
				require.NotNil(t, err)
				require.Equal(t, false, f.s.cfg.ForkChoiceStore.HasNode(d.root))
				f.engine.ErrForkchoiceUpdated = nil
				f.s.UpdateHead(f.ctx, 5)
				synctest.Wait()
				require.Equal(t, failures > 1, d.HasBlock(f.ctx, d.root))
				require.Equal(t, false, f.s.HasBlock(f.ctx, d.root))
				// An engine that is currently syncing cannot reverse an INVALID
				// verdict already established for a beacon ancestor.
				f.engine.ErrNewPayload = execution.ErrAcceptedSyncingPayloadStatus
				f.engine.ErrForkchoiceUpdated = execution.ErrAcceptedSyncingPayloadStatus
				err = f.s.ReceiveBlockBatch(f.ctx, f.blks[2:])
				synctest.Wait()
				published, headErr := f.s.HeadRoot(f.ctx)
				require.NoError(t, headErr)
				assert.NotNil(t, err, "must reject descendants of the already-invalid block")
				assert.Equal(t, false, f.s.cfg.ForkChoiceStore.HasNode(d.root))
				assert.Equal(t, f.blks[0].Root(), bytesutil.ToBytes32(published))
			})
		})
	}
}
