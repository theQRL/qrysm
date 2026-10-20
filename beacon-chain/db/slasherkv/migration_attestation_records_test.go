package slasherkv

import (
	"context"
	"testing"

	slashertypes "github.com/theQRL/qrysm/beacon-chain/slasher/types"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/testing/require"
	bolt "go.etcd.io/bbolt"
)

// A database written before records were keyed by target epoch holds them under
// their data root alone. Opening it moves them, so stored evidence stays
// reachable and a double vote against it is still detected.
func TestMigrateAttestationRecordKeys_MovesRootOnlyRecords(t *testing.T) {
	ctx := context.Background()
	db := setupDB(t)

	rootD := bytesutil.PadTo([]byte("data-D"), 32)
	first := createAttestationWrapper(2, 3, []uint64{1}, rootD)
	// Write the record the old way: index entry plus a root-only record key.
	encRecord, err := encodeAttestationRecord(first)
	require.NoError(t, err)
	require.NoError(t, db.db.Update(func(tx *bolt.Tx) error {
		if err := tx.Bucket(attestationRecordsBucket).Put(first.SigningRoot[:], encRecord); err != nil {
			return err
		}
		key := append(encodeTargetEpoch(3), encodeValidatorIndex(1)...)
		if err := tx.Bucket(attestationDataRootsBucket).Put(key, first.SigningRoot[:]); err != nil {
			return err
		}
		return tx.Bucket(slasherMigrationsBucket).Delete(migrationAttestationRecordKeys)
	}))
	// Before the migration the record is unreachable.
	record, err := db.AttestationRecordForValidator(ctx, 1, 3)
	require.NoError(t, err)
	require.Equal(t, true, record == nil)

	require.NoError(t, migrateAttestationRecordKeys(ctx, db.db))

	record, err = db.AttestationRecordForValidator(ctx, 1, 3)
	require.NoError(t, err)
	require.NotNil(t, record)
	require.DeepEqual(t, []uint64{1}, record.IndexedAttestation.AttestingIndices)
	// The old key is gone and the marker is set, so the scan does not run again.
	require.NoError(t, db.db.View(func(tx *bolt.Tx) error {
		require.Equal(t, true, tx.Bucket(attestationRecordsBucket).Get(first.SigningRoot[:]) == nil)
		require.DeepEqual(t, migrationCompleted, tx.Bucket(slasherMigrationsBucket).Get(migrationAttestationRecordKeys))
		return nil
	}))
	// The migrated evidence detects a double vote.
	double := createAttestationWrapper(2, 3, []uint64{1}, bytesutil.PadTo([]byte("data-E"), 32))
	votes, err := db.CheckAttesterDoubleVotes(ctx, []*slashertypes.IndexedAttestationWrapper{double})
	require.NoError(t, err)
	require.Equal(t, 1, len(votes))
	require.Equal(t, primitives.ValidatorIndex(1), votes[0].ValidatorIndex)
}

// An interruption between pruning the index and pruning the records must not
// strand the records: the next run prunes them even though the index is empty.
func TestPruneAttestationsAtEpoch_PrunesRecordsWhenIndexAlreadyPruned(t *testing.T) {
	ctx := context.Background()
	db := setupDB(t)
	att := createAttestationWrapper(2, 3, []uint64{1}, bytesutil.PadTo([]byte("data-D"), 32))
	require.NoError(t, db.SaveAttestationRecordsForValidators(ctx, []*slashertypes.IndexedAttestationWrapper{att}))
	// Simulate the interruption: the index entry is gone, the record is not.
	require.NoError(t, db.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(attestationDataRootsBucket).Delete(append(encodeTargetEpoch(3), encodeValidatorIndex(1)...))
	}))
	_, err := db.PruneAttestationsAtEpoch(ctx, 3)
	require.NoError(t, err)
	require.NoError(t, db.db.View(func(tx *bolt.Tx) error {
		n := 0
		require.NoError(t, tx.Bucket(attestationRecordsBucket).ForEach(func(_, _ []byte) error {
			n++
			return nil
		}))
		require.Equal(t, 0, n, "expired record stranded behind a pruned index")
		return nil
	}))
}

// A failed initialization (here: a cancelled migration) must release the
// database lock so the database can be opened again.
func TestNewKVStore_FailedMigrationReleasesLock(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := NewKVStore(ctx, dir)
	require.NoError(t, err)
	att := createAttestationWrapper(2, 3, []uint64{1}, bytesutil.PadTo([]byte("data-D"), 32))
	encRecord, err := encodeAttestationRecord(att)
	require.NoError(t, err)
	require.NoError(t, db.db.Update(func(tx *bolt.Tx) error {
		if err := tx.Bucket(attestationRecordsBucket).Put(att.SigningRoot[:], encRecord); err != nil {
			return err
		}
		return tx.Bucket(slasherMigrationsBucket).Delete(migrationAttestationRecordKeys)
	}))
	require.NoError(t, db.Close())

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = NewKVStore(cancelled, dir)
	require.ErrorContains(t, "could not migrate attestation record keys", err)

	reopened, err := NewKVStore(ctx, dir)
	require.NoError(t, err, "the database lock was not released by the failed open")
	require.NoError(t, reopened.Close())
}
