package kv

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	dbIface "github.com/theQRL/qrysm/beacon-chain/db/iface"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/beacon-chain/state/genesis"
	"github.com/theQRL/qrysm/config/params"
	consensusblocks "github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/encoding/ssz/detect"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	bolt "go.etcd.io/bbolt"
)

// SaveGenesisData bootstraps the beaconDB with a given genesis state.
func (s *Store) SaveGenesisData(ctx context.Context, genesisState state.BeaconState) error {
	if err := helpers.ValidateGenesisActiveValidatorCount(genesisState); err != nil {
		return err
	}
	wsb, err := blocks.NewGenesisBlockForState(ctx, genesisState)
	if err != nil {
		return errors.Wrap(err, "could not get genesis block root")
	}
	genesisBlkRoot, err := wsb.Block().HashTreeRoot()
	if err != nil {
		return errors.Wrap(err, "could not get genesis block root")
	}
	// The genesis block is stored with its (empty) execution payload even when
	// the database otherwise stores blinded blocks. Its payload header carries
	// the zero block hash, which the execution client cannot resolve, so a
	// blinded genesis block could not be reconstructed to serve it.
	if err := s.saveBlocks(ctx, []interfaces.ReadOnlySignedBeaconBlock{wsb}, false); err != nil {
		return errors.Wrap(err, "could not save genesis block")
	}
	if err := s.SaveState(ctx, genesisState, genesisBlkRoot); err != nil {
		return errors.Wrap(err, "could not save genesis state")
	}
	if err := s.SaveStateSummary(ctx, &qrysmpb.StateSummary{
		Slot: 0,
		Root: genesisBlkRoot[:],
	}); err != nil {
		return err
	}

	if err := s.SaveHeadBlockRoot(ctx, genesisBlkRoot); err != nil {
		return errors.Wrap(err, "could not save head block root")
	}
	if err := s.SaveGenesisBlockRoot(ctx, genesisBlkRoot); err != nil {
		return errors.Wrap(err, "could not save genesis block root")
	}
	return nil
}

// repairBlindedGenesisBlock rewrites a genesis block that an older release
// stored blinded with its (empty) execution payload. The blinded form cannot be
// served: its payload header carries the zero block hash, which the execution
// client cannot resolve. The regular save path never overwrites an existing
// root, so the entry is replaced directly. It runs when the database is opened.
func (s *Store) repairBlindedGenesisBlock(ctx context.Context) error {
	var root, enc []byte
	if err := s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(blocksBucket)
		root = bytesutil.SafeCopyBytes(bkt.Get(genesisBlockRootKey))
		if len(root) == 0 {
			return nil
		}
		enc = bytesutil.SafeCopyBytes(bkt.Get(root))
		return nil
	}); err != nil {
		return err
	}
	if len(root) == 0 || len(enc) == 0 {
		return nil
	}
	blk, err := unmarshalBlock(ctx, enc)
	if err != nil {
		return errors.Wrap(err, "could not decode the stored genesis block")
	}
	if err := consensusblocks.BeaconBlockIsNil(blk); err != nil {
		return errors.Wrap(err, "stored genesis block")
	}
	if !blk.IsBlinded() {
		return nil
	}
	emptyPayload := blocks.NewGenesisBlock(nil).Block.Body.ExecutionPayload
	full, err := consensusblocks.BuildSignedBeaconBlockFromExecutionPayload(blk, emptyPayload)
	if err != nil {
		// Not the genesis block of this chain's construction; it cannot be
		// served either way, which is no reason to keep the node from starting.
		log.WithError(err).WithField("root", fmt.Sprintf("%#x", root)).
			Error("Stored genesis block is blinded and its execution payload could not be restored")
		return nil
	}
	fullRoot, err := full.Block().HashTreeRoot()
	if err != nil {
		return err
	}
	if fullRoot != bytesutil.ToBytes32(root) {
		log.WithField("root", fmt.Sprintf("%#x", root)).WithField("restoredRoot", fmt.Sprintf("%#x", fullRoot)).
			Error("Stored genesis block is blinded and does not carry the empty execution payload")
		return nil
	}
	encFull, err := marshalBlockFull(ctx, full)
	if err != nil {
		return errors.Wrap(err, "could not encode the restored genesis block")
	}
	s.blockWriteLock.Lock()
	defer s.blockWriteLock.Unlock()
	if err := s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(blocksBucket).Put(root, encFull)
	}); err != nil {
		return errors.Wrap(err, "could not store the restored genesis block")
	}
	s.blockCache.Del(string(root))
	log.WithField("root", fmt.Sprintf("%#x", root)).Info("Restored the execution payload of the stored genesis block")
	return nil
}

// LoadGenesis loads a genesis state from a ssz-serialized byte slice, if no genesis exists already.
func (s *Store) LoadGenesis(ctx context.Context, sb []byte) error {
	if len(sb) < (1 << 10) {
		log.WithField("size", fmt.Sprintf("%d bytes", len(sb))).
			Warn("Genesis state is smaller than one 1Kb. This could be an empty file, git lfs metadata file, or corrupt genesis state.")
	}
	vu, err := detect.FromState(sb)
	if err != nil {
		return err
	}
	gs, err := vu.UnmarshalBeaconState(sb)
	if err != nil {
		return err
	}
	// Validate even when an identical genesis is already stored. Otherwise the
	// no-op path below could accept an oversized genesis from an older release.
	if err := helpers.ValidateGenesisActiveValidatorCount(gs); err != nil {
		return err
	}
	existing, err := s.GenesisState(ctx)
	if err != nil {
		return err
	}
	// If some different genesis state existed already, return an error. The same genesis state is
	// considered a no-op.
	if existing != nil && !existing.IsNil() {
		a, err := existing.HashTreeRoot(ctx)
		if err != nil {
			return err
		}
		b, err := gs.HashTreeRoot(ctx)
		if err != nil {
			return err
		}
		if a == b {
			return nil
		}
		return dbIface.ErrExistingGenesisState
	}

	return s.SaveGenesisData(ctx, gs)
}

// EnsureEmbeddedGenesis checks that a genesis block has been generated when an embedded genesis
// state is used. If a genesis block does not exist, but a genesis state does, then we should call
// SaveGenesisData on the existing genesis state. If a genesis block does exist, it must have been
// produced from the embedded genesis state: GenesisState always returns the embedded state, so a
// database created from a different genesis would otherwise run the stored chain on the wrong state.
func (s *Store) EnsureEmbeddedGenesis(ctx context.Context) error {
	gb, err := s.GenesisBlock(ctx)
	if err != nil {
		return err
	}
	if gb != nil && !gb.IsNil() {
		return s.verifyEmbeddedGenesisMatches(ctx, gb)
	}
	gs, err := s.GenesisState(ctx)
	if err != nil {
		return err
	}
	if gs != nil && !gs.IsNil() {
		return s.SaveGenesisData(ctx, gs)
	}
	return nil
}

// verifyEmbeddedGenesisMatches compares the state root of the stored genesis block with the hash
// tree root of the genesis state embedded for the active config, when there is one.
func (s *Store) verifyEmbeddedGenesisMatches(ctx context.Context, gb interfaces.ReadOnlySignedBeaconBlock) error {
	if err := consensusblocks.BeaconBlockIsNil(gb); err != nil {
		return err
	}
	embedded, err := genesis.State(params.BeaconConfig().ConfigName)
	if err != nil {
		return errors.Wrap(err, "could not load the embedded genesis state")
	}
	if embedded == nil || embedded.IsNil() {
		return nil
	}
	embeddedRoot, err := embedded.HashTreeRoot(ctx)
	if err != nil {
		return errors.Wrap(err, "could not hash the embedded genesis state")
	}
	if stored := gb.Block().StateRoot(); stored != embeddedRoot {
		return errors.Wrapf(dbIface.ErrEmbeddedGenesisMismatch,
			"stored genesis state root %#x, embedded genesis state root %#x; clear the database to start from the embedded genesis",
			stored, embeddedRoot)
	}
	return nil
}
