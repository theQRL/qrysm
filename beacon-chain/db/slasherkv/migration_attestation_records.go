package slasherkv

import (
	"context"
	"encoding/binary"

	"github.com/pkg/errors"
	bolt "go.etcd.io/bbolt"
)

var (
	// slasherMigrationsBucket records completed schema migrations of the slasher database.
	slasherMigrationsBucket = []byte("migrations")
	// migrationAttestationRecordKeys marks the move of attestation records from
	// data-root keys to target-epoch-prefixed keys.
	migrationAttestationRecordKeys = []byte("attestation-record-epoch-keys")
	migrationCompleted             = []byte("done")
	// migrationBatchSize bounds the records moved per transaction.
	migrationBatchSize = 1000
)

// migrateAttestationRecordKeys moves attestation records stored under their data
// root alone (32-byte keys) to keys prefixed with the target epoch, which the
// readers and the pruning now use. Without it an upgraded database would hold
// records no lookup can reach, and the double votes they witness would go
// undetected. A record's target epoch is read from the record itself.
func migrateAttestationRecordKeys(ctx context.Context, db *bolt.DB) error {
	var done bool
	if err := db.View(func(tx *bolt.Tx) error {
		mb := tx.Bucket(slasherMigrationsBucket)
		done = mb != nil && string(mb.Get(migrationAttestationRecordKeys)) == string(migrationCompleted)
		return nil
	}); err != nil {
		return err
	}
	if done {
		return nil
	}

	var oldKeys [][]byte
	if err := db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(attestationRecordsBucket).ForEach(func(k, _ []byte) error {
			if len(k) == signingRootSize {
				oldKeys = append(oldKeys, append([]byte{}, k...))
			}
			return nil
		})
	}); err != nil {
		return err
	}
	if len(oldKeys) > 0 {
		log.WithField("records", len(oldKeys)).Info("Migrating slasher attestation records to epoch-prefixed keys")
	}
	for start := 0; start < len(oldKeys); start += migrationBatchSize {
		if err := ctx.Err(); err != nil {
			return err
		}
		stop := min(start+migrationBatchSize, len(oldKeys))
		if err := db.Update(func(tx *bolt.Tx) error {
			bkt := tx.Bucket(attestationRecordsBucket)
			for _, k := range oldKeys[start:stop] {
				v := bkt.Get(k)
				if v == nil {
					continue
				}
				record, err := decodeAttestationRecord(v)
				if err != nil {
					return errors.Wrapf(err, "could not decode attestation record %#x", k)
				}
				if record.IndexedAttestation.Data == nil || record.IndexedAttestation.Data.Target == nil {
					return errors.Errorf("attestation record %#x has no target", k)
				}
				encEpoch := make([]byte, 8)
				binary.BigEndian.PutUint64(encEpoch, uint64(record.IndexedAttestation.Data.Target.Epoch))
				newKey := attestationRecordKey(encEpoch, k)
				if bkt.Get(newKey) == nil {
					if err := bkt.Put(newKey, append([]byte{}, v...)); err != nil {
						return err
					}
				}
				if err := bkt.Delete(k); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(slasherMigrationsBucket).Put(migrationAttestationRecordKeys, migrationCompleted)
	})
}
