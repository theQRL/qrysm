package stategen

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/theQRL/qrysm/beacon-chain/core/altair"
	qrysmtime "github.com/theQRL/qrysm/beacon-chain/core/time"
	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	"github.com/theQRL/qrysm/beacon-chain/db/filters"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/monitoring/tracing"
	"github.com/theQRL/qrysm/runtime/version"
	"go.opencensus.io/trace"
)

// blockRootGetter retrieves one block by its root. Replaying roots rather than
// retaining decoded blocks keeps the replay's block memory bounded by one block.
type blockRootGetter interface {
	Block(ctx context.Context, blockRoot [32]byte) (interfaces.ReadOnlySignedBeaconBlock, error)
}

// replayBlockRoots replays canonical block roots in ascending slot order. A
// block is fetched immediately before its transition and then released before
// the next root is fetched.
func (s *State) replayBlockRoots(
	ctx context.Context,
	st state.BeaconState,
	roots [][32]byte,
	targetSlot primitives.Slot,
) (state.BeaconState, error) {
	ctx, span := trace.StartSpan(ctx, "stateGen.replayBlockRoots")
	defer span.End()

	start := time.Now()
	diff := primitives.Slot(0)
	if targetSlot >= st.Slot() {
		diff = targetSlot - st.Slot()
	}
	rLog := log.WithFields(logrus.Fields{
		"startSlot": st.Slot(),
		"endSlot":   targetSlot,
		"diff":      diff,
	})
	rLog.Debug("Replaying state")

	st, err := replayBlockRootsWithGetter(ctx, st, roots, targetSlot, s.beaconDB)
	if err != nil {
		return nil, err
	}

	duration := time.Since(start)
	rLog.WithFields(logrus.Fields{
		"duration": duration,
	}).Debug("Replayed state")

	replayBlocksSummary.Observe(float64(duration.Milliseconds()))
	return st, nil
}

func replayBlockRootsWithGetter(
	ctx context.Context,
	st state.BeaconState,
	roots [][32]byte,
	targetSlot primitives.Slot,
	getter blockRootGetter,
) (state.BeaconState, error) {
	if st.Slot() > targetSlot {
		return nil, errors.Wrapf(
			ErrReplayTargetSlotExceeded,
			"state slot %d is greater than replay target slot %d",
			st.Slot(),
			targetSlot,
		)
	}
	var err error
	for _, root := range roots {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		st, err = replayBlockRoot(ctx, st, root, targetSlot, getter)
		if err != nil {
			return nil, err
		}
	}

	// If there are skip slots at the end.
	if targetSlot > st.Slot() {
		st, err = ReplayProcessSlots(ctx, st, targetSlot)
		if err != nil {
			return nil, err
		}
	}
	return st, nil
}

func replayBlockRoot(
	ctx context.Context,
	st state.BeaconState,
	root [32]byte,
	targetSlot primitives.Slot,
	getter blockRootGetter,
) (state.BeaconState, error) {
	signed, err := getter.Block(ctx, root)
	if err != nil {
		return nil, errors.Wrapf(err, "could not retrieve block at root %#x", root)
	}
	if err := blocks.BeaconBlockIsNil(signed); err != nil {
		return nil, errors.Wrapf(err, "could not retrieve block at root %#x", root)
	}
	if signed.Block().Slot() > targetSlot {
		return st, nil
	}
	// A cached ancestor state can be at a later slot than its block root, and a
	// checkpoint state can be a block's post-state advanced through empty
	// slots. A block is skipped only when the state has already applied a block
	// at that slot (its latest block header says so): a state at the block's
	// slot whose header is older is the block's pre-state.
	if st.Slot() > signed.Block().Slot() {
		return st, nil
	}
	if st.Slot() == signed.Block().Slot() && st.LatestBlockHeader() != nil && st.LatestBlockHeader().Slot == signed.Block().Slot() {
		return st, nil
	}
	return executeStateTransitionStateGen(ctx, st, signed)
}

func reverseBlockRoots(roots [][32]byte) {
	for left, right := 0, len(roots)-1; left < right; left, right = left+1, right-1 {
		roots[left], roots[right] = roots[right], roots[left]
	}
}

// executeStateTransitionStateGen replays a previously verified block. It skips proposer,
// attestation and sync committee signature verification, as well as RANDAO reveal verification.
// Other block operations retain their checks.
//
// WARNING: This method should not be used on an unverified new block.
func executeStateTransitionStateGen(
	ctx context.Context,
	state state.BeaconState,
	signed interfaces.ReadOnlySignedBeaconBlock,
) (state.BeaconState, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err := blocks.BeaconBlockIsNil(signed); err != nil {
		return nil, err
	}
	ctx, span := trace.StartSpan(ctx, "stategen.executeStateTransitionStateGen")
	defer span.End()
	var err error

	// Execute per slots transition.
	// Given this is for state gen, a node uses the version of process slots without skip slots cache.
	state, err = ReplayProcessSlots(ctx, state, signed.Block().Slot())
	if err != nil {
		return nil, errors.Wrap(err, "could not process slot")
	}

	// Replay stored blocks without repeating sync committee signature verification.
	state, err = transition.ProcessBlockForReplay(ctx, state, signed)
	if err != nil {
		return nil, errors.Wrap(err, "could not process block")
	}
	return state, nil
}

// ReplayProcessSlots to process old slots for state gen usages.
// There's no skip slot cache involved given state gen only works with already stored block and state in DB.
//
// WARNING: This method should not be used for future slot.
func ReplayProcessSlots(ctx context.Context, state state.BeaconState, slot primitives.Slot) (state.BeaconState, error) {
	ctx, span := trace.StartSpan(ctx, "stategen.ReplayProcessSlots")
	defer span.End()
	if state == nil || state.IsNil() {
		return nil, errUnknownState
	}

	if state.Slot() > slot {
		err := fmt.Errorf("expected state.slot %d <= slot %d", state.Slot(), slot)
		return nil, err
	}

	if state.Slot() == slot {
		return state, nil
	}

	var err error
	for state.Slot() < slot {
		// Slot and epoch transitions never look at the context; a replay for a
		// request that went away would otherwise run to the target slot.
		if err := ctx.Err(); err != nil {
			tracing.AnnotateError(span, err)
			return nil, err
		}
		state, err = transition.ProcessSlot(ctx, state)
		if err != nil {
			return nil, errors.Wrap(err, "could not process slot")
		}
		if qrysmtime.CanProcessEpoch(state) {
			switch state.Version() {
			case version.Zond:
				state, err = altair.ProcessEpoch(ctx, state)
				if err != nil {
					tracing.AnnotateError(span, err)
					return nil, errors.Wrap(err, "could not process epoch")
				}
			default:
				return nil, fmt.Errorf("unsupported beacon state version: %s", version.String(state.Version()))
			}
		}
		if err := state.SetSlot(state.Slot() + 1); err != nil {
			tracing.AnnotateError(span, err)
			return nil, errors.Wrap(err, "failed to increment state slot")
		}
	}

	return state, nil
}

// Given the start slot and the end slot, this returns the finalized beacon blocks in between.
// Since hot states don't have finalized blocks, this should ONLY be used for replaying cold state.
func (s *State) loadFinalizedBlocks(ctx context.Context, startSlot, endSlot primitives.Slot) ([]interfaces.ReadOnlySignedBeaconBlock, error) {
	f := filters.NewFilter().SetStartSlot(startSlot).SetEndSlot(endSlot)
	bs, bRoots, err := s.beaconDB.Blocks(ctx, f)
	if err != nil {
		return nil, err
	}
	if len(bs) != len(bRoots) {
		return nil, errors.New("length of blocks and roots don't match")
	}
	fbs := make([]interfaces.ReadOnlySignedBeaconBlock, 0, len(bs))
	for i := len(bs) - 1; i >= 0; i-- {
		if s.beaconDB.IsFinalizedBlock(ctx, bRoots[i]) {
			fbs = append(fbs, bs[i])
		}
	}
	return fbs, nil
}
