package kv

import (
	"bytes"
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/encoding/ssz/detect"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/runtime/version"
	bolt "go.etcd.io/bbolt"
)

// errOriginBlockMismatch is returned when the checkpoint block is not the block
// recorded in the checkpoint state's latest block header.
var errOriginBlockMismatch = errors.New("checkpoint block does not match the checkpoint state")

// errOriginOnSyncedDatabase is returned when a checkpoint import is attempted on a
// database that already holds finalized chain data beyond genesis.
var errOriginGenesisMismatch = errors.New("checkpoint state does not belong to this chain: its genesis validators root or genesis time differs from the local genesis")

var errOriginOnSyncedDatabase = errors.New("checkpoint sync requires a database without finalized chain data; clear the database to sync from a checkpoint")

// verifyOriginBlockMatchesState checks that the checkpoint block is the block the
// checkpoint state was produced by. A mismatched pair would otherwise be persisted
// as origin, head, justified and finalized checkpoint, after which the node could
// never process a block.
func verifyOriginBlockMatchesState(ctx context.Context, blk interfaces.ReadOnlyBeaconBlock, st state.BeaconState) error {
	hdr := st.LatestBlockHeader()
	if hdr == nil {
		return errors.Wrap(errOriginBlockMismatch, "state has no latest block header")
	}
	bodyRoot, err := blk.Body().HashTreeRoot()
	if err != nil {
		return errors.Wrap(err, "could not compute checkpoint block body root")
	}
	parentRoot := blk.ParentRoot()
	switch {
	case blk.Slot() != hdr.Slot:
		return errors.Wrapf(errOriginBlockMismatch, "block slot %d, state latest block header slot %d", blk.Slot(), hdr.Slot)
	case blk.ProposerIndex() != hdr.ProposerIndex:
		return errors.Wrapf(errOriginBlockMismatch, "block proposer %d, state latest block header proposer %d", blk.ProposerIndex(), hdr.ProposerIndex)
	case !bytes.Equal(parentRoot[:], hdr.ParentRoot):
		return errors.Wrapf(errOriginBlockMismatch, "block parent root %#x, state latest block header parent root %#x", parentRoot, hdr.ParentRoot)
	case !bytes.Equal(bodyRoot[:], hdr.BodyRoot):
		return errors.Wrapf(errOriginBlockMismatch, "block body root %#x, state latest block header body root %#x", bodyRoot, hdr.BodyRoot)
	}
	// The header checks above do not cover the block's state root. A block
	// with a wrong state root has a different root, and persisting it as the
	// origin would leave the real descendants without a parent.
	blockStateRoot := blk.StateRoot()
	switch {
	case st.Slot() == blk.Slot():
		// The checkpoint state is the block's post-state.
		stateRoot, err := st.HashTreeRoot(ctx)
		if err != nil {
			return errors.Wrap(err, "could not hash the checkpoint state")
		}
		if stateRoot != blockStateRoot {
			return errors.Wrapf(errOriginBlockMismatch, "block state root %#x, checkpoint state root %#x", blockStateRoot, stateRoot)
		}
	case st.Slot() > blk.Slot():
		// The checkpoint state was advanced through empty slots after the
		// block. The first slot processed after the block fills the state
		// root of the latest block header with the block's post-state root,
		// however far the state was advanced since.
		if !bytes.Equal(hdr.StateRoot, blockStateRoot[:]) {
			return errors.Wrapf(errOriginBlockMismatch, "block state root %#x, state latest block header state root %#x", blockStateRoot, hdr.StateRoot)
		}
	default:
		return errors.Wrapf(errOriginBlockMismatch, "checkpoint state slot %d is before block slot %d", st.Slot(), blk.Slot())
	}
	return nil
}

// SaveOrigin loads an ssz serialized Block & BeaconState from an io.Reader
// (ex: an open file) prepares the database so that the beacon node can begin
// syncing, using the provided values as their point of origin. This is an alternative
// to syncing from genesis, and should only be run on an empty database.
func (s *Store) SaveOrigin(ctx context.Context, serState, serBlock []byte) error {
	genesisRoot, err := s.GenesisBlockRoot(ctx)
	if err != nil {
		if errors.Is(err, ErrNotFoundGenesisBlockRoot) {
			return errors.Wrap(err, "genesis block root not found: genesis must be provided for checkpoint sync")
		}
		return errors.Wrap(err, "genesis block root query error: checkpoint sync must verify genesis to proceed")
	}
	// A checkpoint can only become the origin of an otherwise empty chain. On a
	// database that already finalized past genesis the finalized index would be
	// rebuilt from an epoch range that runs backwards (and fail after the
	// checkpoint block and state were committed), or the backfill marker would
	// be reset below history the node already holds.
	finalized, err := s.FinalizedCheckpoint(ctx)
	if err != nil {
		return errors.Wrap(err, "could not read the finalized checkpoint")
	}
	if finalized.Epoch > 0 || (bytesutil.ToBytes32(finalized.Root) != params.BeaconConfig().ZeroHash && bytesutil.ToBytes32(finalized.Root) != genesisRoot) {
		return errOriginOnSyncedDatabase
	}
	cf, err := detect.FromState(serState)
	if err != nil {
		return errors.Wrap(err, "could not sniff config+fork for origin state bytes")
	}
	_, ok := params.BeaconConfig().ForkVersionSchedule[cf.Version]
	if !ok {
		return fmt.Errorf("config mismatch, beacon node configured to connect to %s, detected state is for %s", params.BeaconConfig().ConfigName, cf.Config.ConfigName)
	}

	log.Infof("detected supported config for state & block version, config name=%s, fork name=%s", cf.Config.ConfigName, version.String(cf.Fork))
	state, err := cf.UnmarshalBeaconState(serState)
	if err != nil {
		return errors.Wrap(err, "failed to initialize origin state w/ bytes + config+fork")
	}
	if err := helpers.ValidateCheckpointActiveValidatorCount(state); err != nil {
		return errors.Wrap(err, "invalid checkpoint state")
	}
	// The fork version only says which config the state was produced under. The
	// genesis validators root and the genesis time identify the chain, and a
	// checkpoint from another chain with the same config would otherwise be
	// installed as the trusted anchor.
	gs, err := s.GenesisState(ctx)
	if err != nil {
		return errors.Wrap(err, "could not load the genesis state")
	}
	if gs == nil || gs.IsNil() {
		return errors.New("genesis state not found: genesis must be provided for checkpoint sync")
	}
	if !bytes.Equal(state.GenesisValidatorsRoot(), gs.GenesisValidatorsRoot()) || state.GenesisTime() != gs.GenesisTime() {
		return errors.Wrapf(errOriginGenesisMismatch,
			"checkpoint genesis validators root %#x and time %d, local genesis validators root %#x and time %d",
			state.GenesisValidatorsRoot(), state.GenesisTime(), gs.GenesisValidatorsRoot(), gs.GenesisTime())
	}

	wblk, err := cf.UnmarshalBeaconBlock(serBlock)
	if err != nil {
		return errors.Wrap(err, "failed to initialize origin block w/ bytes + config+fork")
	}
	blk := wblk.Block()

	blockRoot, err := blk.HashTreeRoot()
	if err != nil {
		return errors.Wrap(err, "could not compute HashTreeRoot of checkpoint block")
	}
	if err := verifyOriginBlockMatchesState(ctx, blk, state); err != nil {
		return err
	}

	// save block
	log.Infof("saving checkpoint block to db, w/ root=%#x", blockRoot)
	if err := s.SaveBlock(ctx, wblk); err != nil {
		return errors.Wrap(err, "could not save checkpoint block")
	}

	// save state
	log.Infof("calling SaveState w/ blockRoot=%x", blockRoot)
	if err = s.SaveState(ctx, state, blockRoot); err != nil {
		return errors.Wrap(err, "could not save state")
	}
	if err = s.SaveStateSummary(ctx, &qrysmpb.StateSummary{
		Slot: state.Slot(),
		Root: blockRoot[:],
	}); err != nil {
		return errors.Wrap(err, "could not save state summary")
	}

	// rebuild the checkpoint from the block
	// use it to mark the block as justified and finalized
	slotEpoch, err := wblk.Block().Slot().SafeDivSlot(params.BeaconConfig().SlotsPerEpoch)
	if err != nil {
		return err
	}
	chkpt := &qrysmpb.Checkpoint{
		Epoch: primitives.Epoch(slotEpoch),
		Root:  blockRoot[:],
	}
	encChkpt, err := encode(ctx, chkpt)
	if err != nil {
		return errors.Wrap(err, "could not encode checkpoint sync checkpoint")
	}
	hasStateSummary := s.HasStateSummary(ctx, blockRoot)

	// The origin metadata is written in one transaction. A crash after a
	// partial write (origin root and head set, finalized checkpoint still
	// missing) would leave a database that starts from genesis while the
	// backfill status reports the whole pre-origin range as missing, so no block
	// could ever be processed again. With the metadata all-or-nothing, a crash
	// here leaves a database that starts from genesis and the checkpoint import
	// can simply be redone.
	return s.db.Update(func(tx *bolt.Tx) error {
		blocksBkt := tx.Bucket(blocksBucket)
		// genesis root is the initial backfill starting point for checkpoint sync
		if err := blocksBkt.Put(backfillBlockRootKey, genesisRoot[:]); err != nil {
			return errors.Wrap(err, "unable to save genesis root as initial backfill starting point for checkpoint sync")
		}
		// mark block as head of chain, so that processing will pick up from this point
		if err := saveHeadBlockRootTx(tx, blockRoot, hasStateSummary); err != nil {
			return errors.Wrap(err, "could not save head block root")
		}
		// save origin block root in a special key, to be used when the canonical
		// origin (start of chain, ie alternative to genesis) block or state is needed
		if err := blocksBkt.Put(originCheckpointBlockRootKey, blockRoot[:]); err != nil {
			return errors.Wrap(err, "could not save origin block root")
		}
		if err := saveCheckpointTx(ctx, tx, justifiedCheckpointKey, chkpt, encChkpt, hasStateSummary); err != nil {
			return errors.Wrap(err, "could not mark checkpoint sync block as justified")
		}
		if err := s.saveFinalizedCheckpointTx(ctx, tx, chkpt, encChkpt, hasStateSummary); err != nil {
			return errors.Wrap(err, "could not mark checkpoint sync block as finalized")
		}
		// Checkpoint sync starts from an explicitly trusted anchor. Persist that
		// trust instead of inferring execution validity from finality on startup.
		if err := saveCheckpointTx(ctx, tx, lastValidatedCheckpointKey, chkpt, encChkpt, hasStateSummary); err != nil {
			return errors.Wrap(err, "could not mark checkpoint sync block as validated")
		}
		return nil
	})
}
