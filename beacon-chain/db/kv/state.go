package kv

import (
	"bytes"
	"context"
	"fmt"

	"github.com/golang/snappy"
	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/beacon-chain/state/genesis"
	statenative "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	"github.com/theQRL/qrysm/config/features"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/monitoring/tracing"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/time"
	"github.com/theQRL/qrysm/time/slots"
	bolt "go.etcd.io/bbolt"
	"go.opencensus.io/trace"
)

// State returns the saved state using block's signing root,
// this particular block was used to generate the state.
func (s *Store) State(ctx context.Context, blockRoot [32]byte) (state.BeaconState, error) {
	ctx, span := trace.StartSpan(ctx, "BeaconDB.State")
	defer span.End()
	startTime := time.Now()
	// Read the state bytes and its validator entries in one transaction. A
	// state deleted in between (hot state cleanup, invalid block pruning) would
	// otherwise surface as a validator index error instead of the nil state
	// that callers handle.
	var enc []byte
	var valEntries []*qrysmpb.Validator
	var migrationOver bool
	err := s.db.View(func(tx *bolt.Tx) error {
		enc = stateBytesTx(tx, blockRoot)
		if len(enc) == 0 {
			return nil
		}
		var err error
		migrationOver, err = s.isStateValidatorMigrationOverTx(tx)
		if err != nil {
			return err
		}
		valEntries, err = s.validatorEntriesTx(ctx, tx, blockRoot)
		return err
	})
	if err != nil {
		return nil, err
	}
	if len(enc) == 0 {
		return nil, nil
	}

	st, err := unmarshalState(enc, valEntries, migrationOver)
	if err != nil {
		return nil, err
	}
	stateReadingTime.Observe(float64(time.Since(startTime).Milliseconds()))
	return st, err
}

// StateOrError is just like State(), except it only returns a non-error response
// if the requested state is found in the database.
func (s *Store) StateOrError(ctx context.Context, blockRoot [32]byte) (state.BeaconState, error) {
	st, err := s.State(ctx, blockRoot)
	if err != nil {
		return nil, err
	}
	if st == nil || st.IsNil() {
		return nil, errors.Wrap(ErrNotFoundState, fmt.Sprintf("no state with blockroot=%#x", blockRoot))
	}
	return st, nil
}

// GenesisState returns the genesis state in beacon chain.
func (s *Store) GenesisState(ctx context.Context) (state.BeaconState, error) {
	ctx, span := trace.StartSpan(ctx, "BeaconDB.GenesisState")
	defer span.End()

	cached, err := genesis.State(params.BeaconConfig().ConfigName)
	if err != nil {
		tracing.AnnotateError(span, err)
		return nil, err
	}
	span.AddAttributes(trace.BoolAttribute("cache_hit", cached != nil))
	if cached != nil {
		return cached, nil
	}

	var st state.BeaconState
	err = s.db.View(func(tx *bolt.Tx) error {
		// Retrieve genesis block's signing root from blocks bucket,
		// to look up what the genesis state is.
		bucket := tx.Bucket(blocksBucket)
		genesisBlockRoot := bucket.Get(genesisBlockRootKey)

		bucket = tx.Bucket(stateBucket)
		enc := bucket.Get(genesisBlockRoot)
		if enc == nil {
			return nil
		}
		// get the validator entries of the genesis state. Everything is read on
		// this transaction: a second read transaction opened here would wait
		// forever on a writer that needs to grow the file.
		migrationOver, mErr := s.isStateValidatorMigrationOverTx(tx)
		if mErr != nil {
			return mErr
		}
		valEntries, valErr := s.validatorEntriesTx(ctx, tx, bytesutil.ToBytes32(genesisBlockRoot))
		if valErr != nil {
			return valErr
		}

		var uErr error
		st, uErr = unmarshalState(enc, valEntries, migrationOver)
		return uErr
	})
	if err != nil {
		return nil, err
	}
	if st == nil || st.IsNil() {
		return nil, nil
	}
	return st, nil
}

// SaveState stores a state to the db using block's signing root which was used to generate the state.
func (s *Store) SaveState(ctx context.Context, st state.ReadOnlyBeaconState, blockRoot [32]byte) error {
	ctx, span := trace.StartSpan(ctx, "BeaconDB.SaveState")
	defer span.End()
	ok, err := s.isStateValidatorMigrationOver()
	if err != nil {
		return err
	}
	if ok {
		return s.SaveStatesEfficient(ctx, []state.ReadOnlyBeaconState{st}, [][32]byte{blockRoot})
	}
	return s.SaveStates(ctx, []state.ReadOnlyBeaconState{st}, [][32]byte{blockRoot})
}

// SaveStates stores multiple states to the db using the provided corresponding roots.
func (s *Store) SaveStates(ctx context.Context, states []state.ReadOnlyBeaconState, blockRoots [][32]byte) error {
	ctx, span := trace.StartSpan(ctx, "BeaconDB.SaveStates")
	defer span.End()
	if states == nil {
		return errors.New("nil state")
	}
	startTime := time.Now()
	multipleEncs := make([][]byte, len(states))
	for i, st := range states {
		stateBytes, err := marshalState(ctx, st)
		if err != nil {
			return err
		}
		multipleEncs[i] = stateBytes
	}

	if err := s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(stateBucket)
		for i, rt := range blockRoots {
			if err := s.removeStaleSlotIndex(ctx, tx, rt, states[i].Slot()); err != nil {
				return err
			}
			indicesByBucket := createStateIndicesFromStateSlot(ctx, states[i].Slot())
			if err := updateValueForIndices(ctx, indicesByBucket, rt[:], tx); err != nil {
				return errors.Wrap(err, "could not update DB indices")
			}
			if err := bucket.Put(rt[:], multipleEncs[i]); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	stateSavingTime.Observe(float64(time.Since(startTime).Milliseconds()))
	return nil
}

type withValidators interface {
	GetValidators() []*qrysmpb.Validator
}

// SaveStatesEfficient stores multiple states to the db (new schema) using the provided corresponding roots.
func (s *Store) SaveStatesEfficient(ctx context.Context, states []state.ReadOnlyBeaconState, blockRoots [][32]byte) error {
	ctx, span := trace.StartSpan(ctx, "BeaconDB.SaveStatesEfficient")
	defer span.End()
	if states == nil {
		return errors.New("nil state")
	}
	validatorKeys, validatorsEntries, err := getValidators(states)
	if err != nil {
		return err
	}

	var toCache []pendingValidatorEntry
	if err := s.db.Update(func(tx *bolt.Tx) error {
		var err error
		toCache, err = s.saveStatesEfficientInternal(ctx, tx, blockRoots, states, validatorKeys, validatorsEntries)
		return err
	}); err != nil {
		return err
	}
	// Populate the cache only once the entries are durable. A cache entry for
	// a failed commit would make later saves skip writing that validator, and
	// the states referencing it would be unreadable after a restart.
	for _, e := range toCache {
		s.validatorEntryCache.Set(e.key, e.entry, e.cost)
	}
	return nil
}

func getValidators(states []state.ReadOnlyBeaconState) ([][]byte, map[string]*qrysmpb.Validator, error) {
	validatorsEntries := make(map[string]*qrysmpb.Validator) // It's a map to make sure that you store only new validator entries.
	validatorKeys := make([][]byte, len(states))             // For every state, this stores a compressed list of validator keys.
	for i, st := range states {
		pb, ok := st.ToProtoUnsafe().(withValidators)
		if !ok {
			return nil, nil, errors.New("could not cast state to interface with GetValidators()")
		}
		validators := pb.GetValidators()

		// yank out the validators and store them in separate table to save space.
		var hashes []byte
		for _, val := range validators {
			// create the unique hash for that validator entry.
			hash, hashErr := val.HashTreeRoot()
			if hashErr != nil {
				return nil, nil, hashErr
			}
			hashes = append(hashes, hash[:]...)

			// note down the hash and the encoded validator entry
			hashStr := string(hash[:])
			validatorsEntries[hashStr] = val
		}
		validatorKeys[i] = snappy.Encode(nil, hashes)
	}
	return validatorKeys, validatorsEntries, nil
}

// pendingValidatorEntry is a validator entry written in a transaction that
// may be added to the validator entry cache once that transaction committed.
type pendingValidatorEntry struct {
	key   []byte
	entry *qrysmpb.Validator
	cost  int64
}

func (s *Store) saveStatesEfficientInternal(ctx context.Context, tx *bolt.Tx, blockRoots [][32]byte, states []state.ReadOnlyBeaconState, validatorKeys [][]byte, validatorsEntries map[string]*qrysmpb.Validator) ([]pendingValidatorEntry, error) {
	bucket := tx.Bucket(stateBucket)
	valIdxBkt := tx.Bucket(blockRootValidatorHashesBucket)
	for i, rt := range blockRoots {
		if err := s.removeStaleSlotIndex(ctx, tx, rt, states[i].Slot()); err != nil {
			return nil, err
		}
		indicesByBucket := createStateIndicesFromStateSlot(ctx, states[i].Slot())
		if err := updateValueForIndices(ctx, indicesByBucket, rt[:], tx); err != nil {
			return nil, errors.Wrap(err, "could not update DB indices")
		}

		// There is a gap when the states that are passed are used outside this
		// thread. But while storing the state object, we should not store the
		// validator entries.To bring the gap closer, we empty the validators
		// just before Put() and repopulate that state with original validators.
		// look at issue https://github.com/prysmaticlabs/prysm/issues/9262.
		switch rawType := states[i].ToProtoUnsafe().(type) {
		case *qrysmpb.BeaconStateZond:
			pbState, err := getZondPbState(rawType)
			if err != nil {
				return nil, err
			}
			valEntries := pbState.Validators
			pbState.Validators = make([]*qrysmpb.Validator, 0)
			rawObj, err := pbState.MarshalSSZ()
			if err != nil {
				return nil, err
			}
			encodedState := snappy.Encode(nil, append(zondKey, rawObj...))
			if err := bucket.Put(rt[:], encodedState); err != nil {
				return nil, err
			}
			pbState.Validators = valEntries
			if err := valIdxBkt.Put(rt[:], validatorKeys[i]); err != nil {
				return nil, err
			}
		default:
			return nil, errors.New("invalid state type")
		}
	}
	// store the validator entries separately to save space.
	return s.storeValidatorEntriesSeparately(ctx, tx, validatorsEntries)
}

func getZondPbState(rawState any) (*qrysmpb.BeaconStateZond, error) {
	pbState, err := statenative.ProtobufBeaconStateZond(rawState)
	if err != nil {
		return nil, err
	}
	if pbState == nil {
		return nil, errors.New("nil state")
	}
	return pbState, nil
}

// storeValidatorEntriesSeparately writes the validator entries that are in
// neither the cache nor the database and returns them, so the caller can add
// them to the cache after the transaction has committed.
func (s *Store) storeValidatorEntriesSeparately(ctx context.Context, tx *bolt.Tx, validatorsEntries map[string]*qrysmpb.Validator) ([]pendingValidatorEntry, error) {
	valBkt := tx.Bucket(stateValidatorsBucket)
	pending := make([]pendingValidatorEntry, 0)
	for hashStr, validatorEntry := range validatorsEntries {
		key := []byte(hashStr)
		if _, ok := s.validatorEntryCache.Get(key); !ok {
			validatorEntryCacheMiss.Inc()
			if valEntry := valBkt.Get(key); valEntry == nil {
				valBytes, encodeErr := encode(ctx, validatorEntry)
				if encodeErr != nil {
					return nil, encodeErr
				}
				if putErr := valBkt.Put(key, valBytes); putErr != nil {
					return nil, putErr
				}
				pending = append(pending, pendingValidatorEntry{key: key, entry: validatorEntry, cost: int64(len(valBytes))})
			}
		} else {
			validatorEntryCacheHit.Inc()
		}
	}
	return pending, nil
}

// HasState checks if a state by root exists in the db.
func (s *Store) HasState(ctx context.Context, blockRoot [32]byte) bool {
	_, span := trace.StartSpan(ctx, "BeaconDB.HasState")
	defer span.End()
	hasState := false
	err := s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(stateBucket)
		stBytes := bkt.Get(blockRoot[:])
		if len(stBytes) > 0 {
			hasState = true
		}
		return nil
	})
	if err != nil {
		// Only a closed database fails a read-only view, which can happen to a
		// background routine during shutdown. Report no state rather than panic.
		log.WithError(err).Error("Could not check for a state")
		return false
	}
	return hasState
}

// DeleteState by block root.
func (s *Store) DeleteState(ctx context.Context, blockRoot [32]byte) error {
	ctx, span := trace.StartSpan(ctx, "BeaconDB.DeleteState")
	defer span.End()

	return s.db.Update(func(tx *bolt.Tx) error {
		return s.deleteStateTx(ctx, tx, blockRoot)
	})
}

// deleteStateTx is DeleteState within an open write transaction. It refuses to
// delete the genesis, justified and finalized checkpoint states, and is a no-op
// for a root without a state.
func (s *Store) deleteStateTx(ctx context.Context, tx *bolt.Tx, blockRoot [32]byte) error {
	bkt := tx.Bucket(blocksBucket)
	genesisBlockRoot := bkt.Get(genesisBlockRootKey)

	bkt = tx.Bucket(checkpointBucket)
	enc := bkt.Get(finalizedCheckpointKey)
	finalized := &qrysmpb.Checkpoint{}
	if enc == nil {
		finalized = &qrysmpb.Checkpoint{Root: genesisBlockRoot}
	} else if err := decode(ctx, enc, finalized); err != nil {
		return err
	}

	enc = bkt.Get(justifiedCheckpointKey)
	justified := &qrysmpb.Checkpoint{}
	if enc == nil {
		justified = &qrysmpb.Checkpoint{Root: genesisBlockRoot}
	} else if err := decode(ctx, enc, justified); err != nil {
		return err
	}

	bkt = tx.Bucket(stateBucket)
	// Safeguard against deleting genesis, finalized, head state.
	if bytes.Equal(blockRoot[:], finalized.Root) || bytes.Equal(blockRoot[:], genesisBlockRoot) || bytes.Equal(blockRoot[:], justified.Root) {
		return ErrDeleteJustifiedAndFinalized
	}

	// Nothing to delete if state doesn't exist.
	enc = bkt.Get(blockRoot[:])
	if enc == nil {
		return nil
	}

	slot, err := s.slotByBlockRoot(ctx, tx, blockRoot[:])
	if err != nil {
		return err
	}
	indicesByBucket := createStateIndicesFromStateSlot(ctx, slot)
	if err := deleteValueForIndices(ctx, indicesByBucket, blockRoot[:], tx); err != nil {
		return errors.Wrap(err, "could not delete root for DB indices")
	}

	// Read on this transaction: a nested read transaction can deadlock with a
	// writer that needs to grow the file.
	ok, err := s.isStateValidatorMigrationOverTx(tx)
	if err != nil {
		return err
	}
	if ok {
		// remove the validator entry keys for the corresponding state.
		idxBkt := tx.Bucket(blockRootValidatorHashesBucket)
		compressedValidatorHashes := idxBkt.Get(blockRoot[:])
		err = idxBkt.Delete(blockRoot[:])
		if err != nil {
			return err
		}

		// remove the respective validator entries from the cache.
		if len(compressedValidatorHashes) == 0 {
			return errors.Errorf("invalid compressed validator keys length")
		}
		validatorHashes, sErr := snappy.Decode(nil, compressedValidatorHashes)
		if sErr != nil {
			return errors.Wrap(sErr, "failed to uncompress validator keys")
		}
		if len(validatorHashes)%hashLength != 0 {
			return errors.Errorf("invalid validator keys length: %d", len(validatorHashes))
		}
		for i := 0; i < len(validatorHashes); i += hashLength {
			key := validatorHashes[i : i+hashLength]
			s.validatorEntryCache.Del(key)
			validatorEntryCacheDelete.Inc()
		}
	}

	return bkt.Delete(blockRoot[:])
}

// DeleteStates by block roots. A root whose state is protected (the genesis,
// justified or finalized checkpoint state) is skipped so that it does not
// stop the remaining states from being deleted; the first other error is
// returned after every root has been attempted.
func (s *Store) DeleteStates(ctx context.Context, blockRoots [][32]byte) error {
	ctx, span := trace.StartSpan(ctx, "BeaconDB.DeleteStates")
	defer span.End()

	var firstErr error
	for _, r := range blockRoots {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := s.DeleteState(ctx, r)
		switch {
		case err == nil:
		case errors.Is(err, ErrDeleteJustifiedAndFinalized):
			log.WithField("root", fmt.Sprintf("%#x", bytesutil.Trunc(r[:]))).Debug("Skipping deletion of a protected checkpoint state")
		case firstErr == nil:
			firstErr = err
		}
	}

	return firstErr
}

var errUnknownStateEncoding = errors.New("unknown state encoding")

// unmarshalState decodes marshaled state bytes into a versioned state struct type.
// migrationOver tells whether the validator registry is stored separately (and
// is then taken from validatorEntries). It is a parameter rather than looked up
// here because callers decode inside an open transaction, and opening another
// one would deadlock with a writer that needs to grow the file.
func unmarshalState(enc []byte, validatorEntries []*qrysmpb.Validator, migrationOver bool) (state.BeaconState, error) {
	var err error
	enc, err = snappy.Decode(nil, enc)
	if err != nil {
		return nil, err
	}
	if !hasZondKey(enc) {
		return nil, errors.Wrapf(errUnknownStateEncoding, "len=%d", len(enc))
	}

	// Marshal state bytes to zond beacon state.
	protoState := &qrysmpb.BeaconStateZond{}
	if err := protoState.UnmarshalSSZ(enc[len(zondKey):]); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal encoding for zond")
	}
	if migrationOver {
		protoState.Validators = validatorEntries
	}
	return statenative.InitializeFromProtoUnsafeZond(protoState)
}

// marshal versioned state from struct type down to bytes.
func marshalState(ctx context.Context, st state.ReadOnlyBeaconState) ([]byte, error) {
	switch st.ToProtoUnsafe().(type) {
	case *qrysmpb.BeaconStateZond:
		rState, ok := st.ToProtoUnsafe().(*qrysmpb.BeaconStateZond)
		if !ok {
			return nil, errors.New("non valid inner state")
		}
		if rState == nil {
			return nil, errors.New("nil state")
		}
		rawObj, err := rState.MarshalSSZ()
		if err != nil {
			return nil, err
		}
		return snappy.Encode(nil, append(zondKey, rawObj...)), nil
	default:
		return nil, errors.New("invalid inner state")
	}
}

// Retrieve the validator entries for a given block root within the caller's
// transaction. These entries are stored in a separate bucket to reduce state size.
func (s *Store) validatorEntriesTx(ctx context.Context, tx *bolt.Tx, blockRoot [32]byte) ([]*qrysmpb.Validator, error) {
	ok, err := s.isStateValidatorMigrationOverTx(tx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return make([]*qrysmpb.Validator, 0), nil
	}
	ctx, span := trace.StartSpan(ctx, "BeaconDB.validatorEntries")
	defer span.End()
	var validatorEntries []*qrysmpb.Validator
	// get the validator keys from the index bucket
	idxBkt := tx.Bucket(blockRootValidatorHashesBucket)
	valKey := idxBkt.Get(blockRoot[:])
	if len(valKey) == 0 {
		return nil, errors.Errorf("invalid compressed validator keys length")
	}

	// decompress the keys and check if they are of proper length.
	validatorKeys, sErr := snappy.Decode(nil, valKey)
	if sErr != nil {
		return nil, errors.Wrap(sErr, "failed to uncompress validator keys")
	}
	if len(validatorKeys)%hashLength != 0 {
		return nil, errors.Errorf("invalid validator keys length: %d", len(validatorKeys))
	}

	// get the corresponding validator entries from the validator bucket.
	valBkt := tx.Bucket(stateValidatorsBucket)
	for i := 0; i < len(validatorKeys); i += hashLength {
		key := validatorKeys[i : i+hashLength]
		// get the entry bytes from the cache or from the DB.
		v, ok := s.validatorEntryCache.Get(key)
		if ok {
			valEntry, vType := v.(*qrysmpb.Validator)
			if vType {
				validatorEntries = append(validatorEntries, valEntry)
				validatorEntryCacheHit.Inc()
			} else {
				// this should never happen, but anyway it's good to bail out if one happens.
				return nil, errors.New("validator cache does not have proper object type")
			}
		} else {
			// not in cache, so get it from the DB, decode it and add to the entry list.
			valEntryBytes := valBkt.Get(key)
			if len(valEntryBytes) == 0 {
				return nil, errors.New("could not find validator entry")
			}
			encValEntry := &qrysmpb.Validator{}
			decodeErr := decode(ctx, valEntryBytes, encValEntry)
			if decodeErr != nil {
				return nil, errors.Wrap(decodeErr, "failed to decode validator entry keys")
			}
			validatorEntries = append(validatorEntries, encValEntry)
			validatorEntryCacheMiss.Inc()

			// should add here in cache
			s.validatorEntryCache.Set(key, encValEntry, int64(encValEntry.SizeSSZ()))
		}
	}
	return validatorEntries, nil
}

// stateBytesTx retrieves the encoded state of a block root within the caller's
// transaction, copied out of the bolt mmap. It returns nil when there is none.
func stateBytesTx(tx *bolt.Tx, blockRoot [32]byte) []byte {
	stBytes := tx.Bucket(stateBucket).Get(blockRoot[:])
	if len(stBytes) == 0 {
		return nil
	}
	// Due to https://github.com/boltdb/bolt/issues/204, we need to
	// allocate a byte slice separately in the transaction or there
	// is the possibility of a panic when accessing that particular
	// area of memory.
	dst := make([]byte, len(stBytes))
	copy(dst, stBytes)
	return dst
}

// slotByBlockRoot retrieves the corresponding slot of the input block root.
func (s *Store) slotByBlockRoot(ctx context.Context, tx *bolt.Tx, blockRoot []byte) (primitives.Slot, error) {
	ctx, span := trace.StartSpan(ctx, "BeaconDB.slotByBlockRoot")
	defer span.End()

	bkt := tx.Bucket(stateSummaryBucket)
	enc := bkt.Get(blockRoot)

	if enc == nil {
		// Fall back to check the block.
		bkt := tx.Bucket(blocksBucket)
		enc := bkt.Get(blockRoot)

		if enc == nil {
			// Fallback and check the state.
			bkt = tx.Bucket(stateBucket)
			enc = bkt.Get(blockRoot)
			if enc == nil {
				return 0, errors.New("state enc can't be nil")
			}
			// no need to construct the validator entries as it is not used here.
			migrationOver, err := s.isStateValidatorMigrationOverTx(tx)
			if err != nil {
				return 0, err
			}
			s, err := unmarshalState(enc, nil, migrationOver)
			if err != nil {
				return 0, err
			}
			if s == nil || s.IsNil() {
				return 0, errors.New("state can't be nil")
			}
			return s.Slot(), nil
		}

		blk, err := unmarshalBlock(ctx, enc)
		if err != nil {
			return 0, errors.Wrap(err, "could not unmarshal block")
		}
		if err := blocks.BeaconBlockIsNil(blk); err != nil {
			return 0, err
		}
		return blk.Block().Slot(), nil
	}
	stateSummary := &qrysmpb.StateSummary{}
	if err := decode(ctx, enc, stateSummary); err != nil {
		return 0, errors.Wrap(err, "could not unmarshal state summary")
	}
	return stateSummary.Slot, nil
}

// HighestSlotStatesBelow returns the states with the highest slot below the input slot
// from the db. Ideally there should just be one state per slot, but given validator
// can double propose, a single slot could have multiple block roots and
// results states. This returns a list of states.
func (s *Store) HighestSlotStatesBelow(ctx context.Context, slot primitives.Slot) ([]state.ReadOnlyBeaconState, error) {
	ctx, span := trace.StartSpan(ctx, "BeaconDB.HighestSlotStatesBelow")
	defer span.End()

	var best []byte
	if err := s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(stateSlotIndicesBucket)
		c := bkt.Cursor()
		for s, root := c.First(); s != nil; s, root = c.Next() {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			key := bytesutil.BytesToSlotBigEndian(s)
			if root == nil {
				continue
			}
			if key >= slot {
				break
			}
			// The value is only valid for the life of the transaction.
			best = bytesutil.SafeCopyBytes(root)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	// A slot holds one root per state saved at it (forks); return every one of
	// them, not the first in the packed list.
	states := make([]state.ReadOnlyBeaconState, 0, 1)
	if best != nil {
		roots, err := splitRoots(best)
		if err != nil {
			return nil, errors.Wrap(err, "corrupt value in state slot index")
		}
		for _, root := range roots {
			st, err := s.State(ctx, root)
			if err != nil {
				return nil, err
			}
			if st != nil && !st.IsNil() {
				states = append(states, st)
			}
		}
	}
	if len(states) == 0 {
		st, err := s.GenesisState(ctx)
		if err != nil {
			return nil, err
		}
		states = append(states, st)
	}

	return states, nil
}

// removeStaleSlotIndex removes root from the slot-index entry of a state saved
// earlier under the same root at another slot, when the state summary records
// that slot. A root maps to one post-state slot, so no path the node takes
// replaces a state at a different slot; the guard keeps the index free of an
// entry that no state backs, which the dirty-state cleanup would otherwise
// follow to delete the live state by root.
func (s *Store) removeStaleSlotIndex(ctx context.Context, tx *bolt.Tx, root [32]byte, slot primitives.Slot) error {
	if tx.Bucket(stateBucket).Get(root[:]) == nil {
		return nil
	}
	var previous primitives.Slot
	if cached := s.stateSummaryCache.get(root); cached != nil {
		previous = cached.Slot
	} else if enc := tx.Bucket(stateSummaryBucket).Get(root[:]); len(enc) > 0 {
		summary := &qrysmpb.StateSummary{}
		if err := decode(ctx, enc, summary); err != nil {
			return errors.Wrap(err, "could not decode the state summary of a replaced state")
		}
		previous = summary.Slot
	} else {
		return nil
	}
	if previous == slot {
		return nil
	}
	if err := deleteValueForIndices(ctx, createStateIndicesFromStateSlot(ctx, previous), root[:], tx); err != nil {
		return errors.Wrap(err, "could not remove the stale slot index of a replaced state")
	}
	return nil
}

// createStateIndicesFromStateSlot takes in a state slot and returns
// a map of bolt DB index buckets corresponding to each particular key for indices for
// data, such as (shard indices bucket -> shard 5).
func createStateIndicesFromStateSlot(ctx context.Context, slot primitives.Slot) map[string][]byte {
	_, span := trace.StartSpan(ctx, "BeaconDB.createStateIndicesFromState")
	defer span.End()
	indicesByBucket := make(map[string][]byte)
	// Every index has a unique bucket for fast, binary-search
	// range scans for filtering across keys.
	buckets := [][]byte{
		stateSlotIndicesBucket,
	}

	indices := [][]byte{
		bytesutil.SlotToBytesBigEndian(slot),
	}
	for i := range buckets {
		indicesByBucket[string(buckets[i])] = indices[i]
	}
	return indicesByBucket
}

// CleanUpDirtyStates removes states in DB that falls to under archived point interval rules.
// Only following states would be kept:
// 1.) state_slot % archived_interval == 0. (e.g. archived_interval=2048, states with slot 2048, 4096... etc)
// 2.) archived_interval - archived_interval/3 < state_slot % archived_interval
//
//	(e.g. archived_interval=2048, states with slots after 1365).
//	This is to tolerate skip slots. Not every state lays on the boundary.
//
// 3.) state with current finalized root
// 4.) unfinalized States
// 5.) not origin root
func (s *Store) CleanUpDirtyStates(ctx context.Context, slotsPerArchivedPoint primitives.Slot) error {
	ctx, span := trace.StartSpan(ctx, "BeaconDB. CleanUpDirtyStates")
	defer span.End()

	// The interval divides slots below; refuse zero instead of panicking in
	// the goroutine that runs this at startup.
	if slotsPerArchivedPoint == 0 {
		return errors.New("slots per archived point must be positive")
	}

	f, err := s.FinalizedCheckpoint(ctx)
	if err != nil {
		return err
	}
	finalizedSlot, err := slots.EpochStart(f.Epoch)
	if err != nil {
		return err
	}
	deletedRoots := make([][32]byte, 0)

	oRoot, err := s.OriginCheckpointBlockRoot(ctx)
	if err != nil && !errors.Is(err, ErrNotFoundOriginBlockRoot) {
		// If the node did not use checkpoint sync, there will be no origin block root.
		// Use zero hash which will never match any actual state root.
		return err
	}

	err = s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(stateSlotIndicesBucket)
		return bkt.ForEach(func(k, v []byte) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			slot := bytesutil.BytesToSlotBigEndian(k)
			mod := slot % slotsPerArchivedPoint

			if mod == 0 {
				return nil
			}

			if mod > slotsPerArchivedPoint-slotsPerArchivedPoint/3 {
				return nil
			}

			if slot > finalizedSlot {
				return nil
			}

			// A slot can index more than one state when the chain forked at
			// that slot; every one of them is subject to the rules above.
			roots, err := splitRoots(v)
			if err != nil {
				return errors.Wrapf(err, "corrupt value in state slot index for slot=%d", slot)
			}
			for _, root := range roots {
				if bytesutil.ToBytes32(f.Root) == root {
					continue
				}
				if oRoot == root {
					continue
				}
				archive, err := isArchivedPointState(tx, slot, root, slotsPerArchivedPoint, finalizedSlot)
				if err != nil {
					return err
				}
				if archive {
					continue
				}
				deletedRoots = append(deletedRoots, root)
			}
			return nil
		})
	})
	if err != nil {
		return err
	}

	// Length of to be deleted roots is 0. Nothing to do.
	if len(deletedRoots) == 0 {
		return nil
	}

	log.WithField("count", len(deletedRoots)).Info("Cleaning up dirty states")
	if err := s.DeleteStates(ctx, deletedRoots); err != nil {
		return err
	}

	return err
}

// isArchivedPointState reports whether the state of the block with the given
// root at slot is the archive of the next archived point. The archive of a
// point is the state of the highest canonical block at or below it, so when no
// block was produced at the point itself the archive sits below it, at a slot
// the interval rules above would otherwise clean up; it is never written again
// once the migration has passed the point. The decision needs the finalized
// index up to the point, so a state below a point that finality has not reached
// yet is kept until it has.
func isArchivedPointState(tx *bolt.Tx, slot primitives.Slot, root [32]byte, interval, finalizedSlot primitives.Slot) (bool, error) {
	point := slot + (interval - slot%interval)
	if point > finalizedSlot {
		return true, nil
	}
	finalizedBkt := tx.Bucket(finalizedBlockRootsIndexBucket)
	if finalizedBkt.Get(root[:]) == nil {
		return false, nil
	}
	// A canonical block in (slot, point] would be the archive instead.
	c := tx.Bucket(blockSlotIndicesBucket).Cursor()
	k, v := c.Seek(bytesutil.SlotToBytesBigEndian(point))
	if k == nil {
		k, v = c.Last()
	}
	for ; k != nil; k, v = c.Prev() {
		indexed := bytesutil.BytesToSlotBigEndian(k)
		if indexed > point {
			continue
		}
		if indexed <= slot {
			break
		}
		roots, err := splitRoots(v)
		if err != nil {
			return false, errors.Wrapf(err, "corrupt value in block slot index for slot=%d", indexed)
		}
		for _, r := range roots {
			if finalizedBkt.Get(r[:]) != nil {
				return false, nil
			}
		}
	}
	return true, nil
}

func (s *Store) isStateValidatorMigrationOver() (bool, error) {
	// if flag is enabled, then always follow the new code path.
	if features.Get().EnableHistoricalSpaceRepresentation {
		return true, nil
	}

	// if the flag is not enabled, but the migration is over, then
	// follow the new code path as if the flag is enabled.
	returnFlag := false
	if err := s.db.View(func(tx *bolt.Tx) error {
		var err error
		returnFlag, err = s.isStateValidatorMigrationOverTx(tx)
		return err
	}); err != nil {
		return returnFlag, err
	}
	return returnFlag, nil
}

// isStateValidatorMigrationOverTx is isStateValidatorMigrationOver within an
// open transaction.
func (s *Store) isStateValidatorMigrationOverTx(tx *bolt.Tx) (bool, error) {
	if features.Get().EnableHistoricalSpaceRepresentation {
		return true, nil
	}
	b := tx.Bucket(migrationsBucket).Get(migrationStateValidatorsKey)
	return bytes.Equal(b, migrationCompleted), nil
}
