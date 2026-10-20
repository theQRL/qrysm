package blockchain

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/theQRL/qrysm/beacon-chain/cache"
	forktypes "github.com/theQRL/qrysm/beacon-chain/forkchoice/types"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/testing/require"
)

type syncCommitteeStateGate struct {
	once             sync.Once
	entered, release chan struct{}
}

type gatedSyncCommitteeState struct {
	state.BeaconState
	gate *syncCommitteeStateGate
}

func (s *gatedSyncCommitteeState) Copy() state.BeaconState {
	return &gatedSyncCommitteeState{BeaconState: s.BeaconState.Copy(), gate: s.gate}
}

func (s *gatedSyncCommitteeState) HashTreeRoot(ctx context.Context) ([32]byte, error) {
	s.gate.once.Do(func() {
		close(s.gate.entered)
		<-s.gate.release
	})
	return s.BeaconState.HashTreeRoot(ctx)
}

func TestHeadSyncCommittee_Reorg(t *testing.T) {
	setupEpochTransitionTest(t)
	cfg := params.BeaconConfig().Copy()
	cfg.EpochsPerSyncCommitteePeriod = 2
	params.OverrideBeaconConfig(cfg)
	for _, inFlight := range []bool{false, true} {
		t.Run(map[bool]string{false: "cached old head", true: "old lookup finishes after reorg"}[inFlight], func(t *testing.T) {
			syncCommitteeHeadStateCache = cache.NewSyncCommitteeHeadState()
			t.Cleanup(func() { syncCommitteeHeadStateCache = cache.NewSyncCommitteeHeadState() })
			f := newBatchExecutionFixture(t, 2)
			f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
			require.NoError(t, f.s.cfg.ForkChoiceStore.UpdateJustifiedCheckpoint(f.ctx, &forktypes.Checkpoint{Root: f.s.originBlockRoot}))
			// Skipping slot 2 on B changes its RANDAO history and the next
			// committee. Empty blocks leave both branches unfinalized.
			aState, bState := f.states[2].Copy(), f.states[1].Copy()
			var aBranch, bBranch []blocks.ROBlock
			for slot := primitives.Slot(3); slot <= 22; slot++ {
				a, nextA := emptyBranchBlock(t, f, aState, slot, 'a')
				b, nextB := emptyBranchBlock(t, f, bState, slot, 'b')
				aBranch, bBranch = append(aBranch, a), append(bBranch, b)
				aState, bState = nextA, nextB
			}
			aCommittee, err := aState.NextSyncCommittee()
			require.NoError(t, err)
			bCommittee, err := bState.NextSyncCommittee()
			require.NoError(t, err)
			require.DeepNotEqual(t, aCommittee.Pubkeys, bCommittee.Pubkeys)
			driftGenesisTime(f.s, 23, 0)
			require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, aBranch))
			root, err := f.s.HeadRoot(f.ctx)
			require.NoError(t, err)
			require.Equal(t, aBranch[len(aBranch)-1].Root(), bytesutil.ToBytes32(root))

			type lookupResult struct {
				keys [][]byte
				err  error
			}
			var results chan lookupResult
			var release func()
			if inFlight {
				gate := &syncCommitteeStateGate{entered: make(chan struct{}), release: make(chan struct{})}
				var releaseOnce sync.Once
				release = func() { releaseOnce.Do(func() { close(gate.release) }) }
				t.Cleanup(release)
				f.s.headLock.Lock()
				f.s.head.state = &gatedSyncCommitteeState{BeaconState: f.s.head.state, gate: gate}
				f.s.headLock.Unlock()
				results = make(chan lookupResult, 1)
				go func() {
					keys, err := f.s.HeadSyncCommitteePubKeys(f.ctx, 23, 0)
					results <- lookupResult{keys: keys, err: err}
				}()
				select {
				case <-gate.entered:
				case <-time.After(5 * time.Second):
					t.Fatal("old-head lookup did not reach slot processing")
				}
			} else {
				before, err := f.s.HeadSyncCommitteePubKeys(f.ctx, 23, 0)
				require.NoError(t, err)
				require.DeepEqual(t, aCommittee.Pubkeys[:len(before)], before)
			}

			require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, bBranch))
			newRoot := bBranch[len(bBranch)-1].Root()
			// Give B the votes so the test does not depend on root tie-breaking.
			indices := make([]uint64, bState.NumValidators())
			for i := range indices {
				indices[i] = uint64(i)
			}
			f.s.cfg.ForkChoiceStore.Lock()
			f.s.cfg.ForkChoiceStore.ProcessAttestation(f.ctx, indices, newRoot, 3)
			f.s.cfg.ForkChoiceStore.Unlock()
			f.s.UpdateHead(f.ctx, 23)
			root, err = f.s.HeadRoot(f.ctx)
			require.NoError(t, err)
			require.Equal(t, newRoot, bytesutil.ToBytes32(root))
			if inFlight {
				release()
				result := <-results
				require.NoError(t, result.err)
				require.DeepEqual(t, aCommittee.Pubkeys[:len(result.keys)], result.keys, "the overlapping lookup retains its original snapshot")
			}
			after, err := f.s.HeadSyncCommitteePubKeys(f.ctx, 23, 0)
			require.NoError(t, err)
			require.DeepEqual(t, bCommittee.Pubkeys[:len(after)], after, "new lookups must use the published head's committee")
			for i := 0; i < bState.NumValidators(); i++ {
				key := bState.PubkeyAtIndex(primitives.ValidatorIndex(i))
				var wanted []primitives.CommitteeIndex
				for position, pubkey := range bCommittee.Pubkeys {
					if bytes.Equal(key[:], pubkey) {
						wanted = append(wanted, primitives.CommitteeIndex(position))
					}
				}
				got, err := f.s.HeadSyncCommitteeIndices(f.ctx, primitives.ValidatorIndex(i), 23)
				require.NoError(t, err)
				require.Equal(t, true, samePositions(wanted, got), "committee positions for validator %d", i)
			}
		})
	}
}

func TestHeadSyncCommittee_PreviousSlotAfterHeadAdvance(t *testing.T) {
	setupEpochTransitionTest(t)
	cfg := params.BeaconConfig().Copy()
	cfg.EpochsPerSyncCommitteePeriod = 2
	params.OverrideBeaconConfig(cfg)
	for _, tc := range []struct {
		name string
		slot primitives.Slot
	}{
		{name: "same period", slot: 9},
		{name: "period boundary", slot: 11},
	} {
		t.Run(tc.name, func(t *testing.T) {
			syncCommitteeHeadStateCache = cache.NewSyncCommitteeHeadState()
			t.Cleanup(func() { syncCommitteeHeadStateCache = cache.NewSyncCommitteeHeadState() })
			f := newBatchExecutionFixture(t, int(tc.slot)+1)
			f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
			driftGenesisTime(f.s, int64(tc.slot), 0)
			require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[2:tc.slot]))
			require.Equal(t, tc.slot, f.s.HeadSlot())
			before, err := f.s.HeadSyncCommitteePubKeys(f.ctx, tc.slot, 0)
			require.NoError(t, err)
			positions := make([][]primitives.CommitteeIndex, f.states[tc.slot].NumValidators())
			for i := range positions {
				positions[i], err = f.s.HeadSyncCommitteeIndices(f.ctx, primitives.ValidatorIndex(i), tc.slot)
				require.NoError(t, err)
			}
			// A message from the previous slot remains valid after importing
			// the next block, including when that block rotates committees.
			driftGenesisTime(f.s, int64(tc.slot+1), 0)
			require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[tc.slot:]))
			require.Equal(t, tc.slot+1, f.s.HeadSlot())
			after, err := f.s.HeadSyncCommitteePubKeys(f.ctx, tc.slot, 0)
			require.NoError(t, err)
			require.DeepEqual(t, before, after)
			for i, wanted := range positions {
				got, err := f.s.HeadSyncCommitteeIndices(f.ctx, primitives.ValidatorIndex(i), tc.slot)
				require.NoError(t, err)
				require.Equal(t, true, samePositions(wanted, got), "previous-slot committee positions for validator %d", i)
			}
		})
	}
}
