// Package kv defines a bolt-db, key-value store implementation
// of the Database interface defined by a Qrysm beacon node.
package kv

import (
	"context"
	"fmt"
	"os"
	"path"
	"sync"
	"time"

	"github.com/dgraph-io/ristretto"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	prombolt "github.com/prysmaticlabs/prombbolt"
	"github.com/theQRL/qrysm/beacon-chain/db/iface"
	"github.com/theQRL/qrysm/config/features"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/io/file"
	bolt "go.etcd.io/bbolt"
)

var _ iface.Database = (*Store)(nil)

// OpenTimeout is how long NewKVStore retries the database file lock before
// giving up (bolt retries it every 50 ms). A tool that replaces the database
// file relies on it: holding the lock for longer than this after the file is
// replaced sees off every node that was waiting for the lock.
const OpenTimeout = 1 * time.Second

const (
	// NumOfValidatorEntries is the size of the validator cache entries.
	// we expect to hold a max of 200K validators, so setting it to 2 million (10x the capacity).
	NumOfValidatorEntries = 1 << 21
	// ValidatorEntryMaxCost bounds the validator entry cache by the encoded size
	// of its entries. A QRL validator entry carries a 2592-byte ML-DSA-87 public
	// key and encodes to about 2.7 KB (the upstream value of 64 MiB assumed
	// ~121-byte entries and would hold only ~25k QRL validators, after which every
	// state read decodes the whole registry from disk). 512 MiB holds the ~200K
	// validators NumOfValidatorEntries is sized for.
	ValidatorEntryMaxCost = 1 << 29
	// BeaconNodeDbDirName is the name of the directory containing the beacon node database.
	BeaconNodeDbDirName = "beaconchaindata"
	// DatabaseFileName is the name of the beacon node database.
	DatabaseFileName = "beaconchain.db"

	boltAllocSize = 8 * 1024 * 1024
	// The size of hash length in bytes
	hashLength = 32
	// Specifies the initial mmap size of bolt.
	mmapSize = 536870912
)

var (
	// Metrics for the validator cache.
	validatorEntryCacheHit = promauto.NewCounter(prometheus.CounterOpts{
		Name: "validator_entry_cache_hit_total",
		Help: "The total number of cache hits on the validator entry cache.",
	})
	validatorEntryCacheMiss = promauto.NewCounter(prometheus.CounterOpts{
		Name: "validator_entry_cache_miss_total",
		Help: "The total number of cache misses on the validator entry cache.",
	})
	validatorEntryCacheDelete = promauto.NewCounter(prometheus.CounterOpts{
		Name: "validator_entry_cache_delete_total",
		Help: "The total number of cache deletes on the validator entry cache.",
	})
	stateReadingTime = promauto.NewSummary(prometheus.SummaryOpts{
		Name: "db_beacon_state_reading_milliseconds",
		Help: "Milliseconds it takes to read a beacon state from the DB",
	})
	stateSavingTime = promauto.NewSummary(prometheus.SummaryOpts{
		Name: "db_beacon_state_saving_milliseconds",
		Help: "Milliseconds it takes to save a beacon state to the DB",
	})
)

// BlockCacheSize bounds the block cache by the encoded size of the cached
// blocks. QRL blocks carry ML-DSA-87 signatures (4627 bytes each, one per
// attester) and run to a few megabytes, so the upstream value of 2 MiB (sized
// for ~1000 Ethereum blocks) held one or two blocks and left Block() reads
// effectively uncached. 128 MiB keeps roughly the last epoch of blocks.
var BlockCacheSize = int64(128 << 20)

// blockedBuckets represents the buckets that we want to restrict
// from our metrics fetching for performance reasons. For a detailed
// summary, it can be read in https://github.com/prysmaticlabs/prysm/issues/8274.
var blockedBuckets = [][]byte{
	blocksBucket,
	stateSummaryBucket,
	blockParentRootIndicesBucket,
	blockSlotIndicesBucket,
	finalizedBlockRootsIndexBucket,
}

// Store defines an implementation of the Qrysm Database interface
// using BoltDB as the underlying persistent kv-store for QRL Beacon Nodes.
type Store struct {
	db                  *bolt.DB
	databasePath        string
	blockCache          *ristretto.Cache
	validatorEntryCache *ristretto.Cache
	stateSummaryCache   *stateSummaryCache
	ctx                 context.Context
	// blockWriteLock serializes a block save with a block deletion, each
	// together with its cache update. The cache is populated after a save
	// commits; a deletion that ran in between would otherwise leave the cache
	// serving a block the database no longer holds, and SaveBlock would then
	// skip persisting it again.
	blockWriteLock sync.Mutex
}

// KVStoreDatafilePath is the canonical construction of a full
// database file path from the directory path, so that code outside
// this package can find the full path in a consistent way.
func KVStoreDatafilePath(dirPath string) string {
	return path.Join(dirPath, DatabaseFileName)
}

var Buckets = [][]byte{
	attestationsBucket,
	blocksBucket,
	stateBucket,
	proposerSlashingsBucket,
	attesterSlashingsBucket,
	voluntaryExitsBucket,
	chainMetadataBucket,
	checkpointBucket,
	executionChainBucket,
	stateSummaryBucket,
	stateValidatorsBucket,
	// Indices buckets.
	attestationHeadBlockRootBucket,
	attestationSourceRootIndicesBucket,
	attestationSourceEpochIndicesBucket,
	attestationTargetRootIndicesBucket,
	attestationTargetEpochIndicesBucket,
	blockSlotIndicesBucket,
	stateSlotIndicesBucket,
	blockParentRootIndicesBucket,
	finalizedBlockRootsIndexBucket,
	blockRootValidatorHashesBucket,
	// State management service bucket.
	newStateServiceCompatibleBucket,
	// Migrations
	migrationsBucket,

	feeRecipientBucket,
	registrationBucket,
}

// NewKVStore initializes a new boltDB key-value store at the directory
// path specified, creates the kv-buckets based on the schema, and stores
// an open connection db object as a property of the Store struct.
func NewKVStore(ctx context.Context, dirPath string) (*Store, error) {
	hasDir, err := file.HasDir(dirPath)
	if err != nil {
		return nil, err
	}
	if !hasDir {
		if err := file.MkdirAll(dirPath); err != nil {
			return nil, err
		}
	}
	datafile := KVStoreDatafilePath(dirPath)
	log.Infof("Opening Bolt DB at %s", datafile)
	boltDB, err := bolt.Open(
		datafile,
		params.BeaconIoConfig().ReadWritePermissions,
		&bolt.Options{
			Timeout:         OpenTimeout,
			InitialMmapSize: mmapSize,
		},
	)
	if err != nil {
		if errors.Is(err, bolt.ErrTimeout) {
			return nil, errors.New("cannot obtain database lock, database may be in use by another process")
		}
		return nil, err
	}
	boltDB.AllocSize = boltAllocSize
	// An initialization failure must release the file lock, or the database
	// cannot be reopened until the process exits.
	closeOnError := func(initErr error) error {
		if closeErr := boltDB.Close(); closeErr != nil {
			log.WithError(closeErr).Error("Could not close the database after a failed initialization")
		}
		return initErr
	}
	blockCache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1000,           // number of keys to track frequency of (1000).
		MaxCost:     BlockCacheSize, // maximum cost of cache (1000 Blocks).
		BufferItems: 64,             // number of keys per Get buffer.
	})
	if err != nil {
		return nil, closeOnError(err)
	}

	validatorCache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: NumOfValidatorEntries, // number of entries in cache (2 Million).
		MaxCost:     ValidatorEntryMaxCost, // maximum size of the cache (64Mb)
		BufferItems: 64,                    // number of keys per Get buffer.
	})
	if err != nil {
		return nil, closeOnError(err)
	}

	kv := &Store{
		db:                  boltDB,
		databasePath:        dirPath,
		blockCache:          blockCache,
		validatorEntryCache: validatorCache,
		stateSummaryCache:   newStateSummaryCache(),
		ctx:                 ctx,
	}
	if err := kv.db.Update(func(tx *bolt.Tx) error {
		return createBuckets(tx, Buckets...)
	}); err != nil {
		return nil, closeOnError(err)
	}
	if err = prometheus.Register(createBoltCollector(kv.db)); err != nil {
		return nil, closeOnError(err)
	}
	// Setup the type of block storage used depending on whether or not this is a fresh database.
	if err := kv.setupBlockStorageType(ctx); err != nil {
		prometheus.Unregister(createBoltCollector(kv.db))
		return nil, closeOnError(err)
	}
	// A genesis block stored blinded by an older release cannot be served.
	if err := kv.repairBlindedGenesisBlock(ctx); err != nil {
		prometheus.Unregister(createBoltCollector(kv.db))
		return nil, closeOnError(errors.Wrap(err, "could not repair the stored genesis block"))
	}

	return kv, nil
}

// ClearDB removes the previously stored database in the data directory.
func (s *Store) ClearDB() error {
	if _, err := os.Stat(s.databasePath); os.IsNotExist(err) {
		return nil
	}
	prometheus.Unregister(createBoltCollector(s.db))
	// Close the bolt DB before removing the underlying file so we don't
	// race an unmap with file deletion (matters on Windows; defensive on Linux).
	if err := s.db.Close(); err != nil {
		return errors.Wrap(err, "could not close database before clear")
	}
	if err := os.Remove(path.Join(s.databasePath, DatabaseFileName)); err != nil {
		return errors.Wrap(err, "could not remove database file")
	}
	return nil
}

// Close closes the underlying BoltDB database.
func (s *Store) Close() error {
	prometheus.Unregister(createBoltCollector(s.db))

	// Before DB closes, we should dump the cached state summary objects to DB.
	if err := s.saveCachedStateSummariesDB(s.ctx); err != nil {
		return err
	}

	return s.db.Close()
}

// DatabasePath at which this database writes files.
func (s *Store) DatabasePath() string {
	return s.databasePath
}

func (s *Store) setupBlockStorageType(ctx context.Context) error {
	// We check if we want to save blinded beacon blocks by checking a key in the db
	// otherwise, we check the last stored block and set that key in the DB if it is blinded.
	headBlock, err := s.HeadBlock(ctx)
	if err != nil {
		return errors.Wrap(err, "could not get head block when setting up block storage type")
	}
	err = blocks.BeaconBlockIsNil(headBlock)
	isNilBlk := err != nil
	saveFull := features.Get().SaveFullExecutionPayloads

	var saveBlinded bool
	if err := s.db.Update(func(tx *bolt.Tx) error {
		// If we have a key stating we wish to save blinded beacon blocks, then we set saveBlinded to true.
		metadataBkt := tx.Bucket(chainMetadataBucket)
		keyExists := len(metadataBkt.Get(saveBlindedBeaconBlocksKey)) > 0
		if keyExists {
			saveBlinded = true
			return nil
		}
		// If the head block exists and is blinded, we update the key in the DB to
		// say we wish to save all blocks as blinded.
		if !isNilBlk && headBlock.IsBlinded() {
			if err := metadataBkt.Put(saveBlindedBeaconBlocksKey, []byte{1}); err != nil {
				return err
			}
			saveBlinded = true
		}
		if isNilBlk && !saveFull {
			if err := metadataBkt.Put(saveBlindedBeaconBlocksKey, []byte{1}); err != nil {
				return err
			}
			saveBlinded = true
		}
		return nil
	}); err != nil {
		return err
	}

	// If the user wants to save full execution payloads but their database is saving blinded blocks only,
	// we then throw an error as the node should not start.
	if saveFull && saveBlinded {
		// TODO(now.youtrack.cloud/issue/TQ-1)
		return fmt.Errorf(
			"cannot use the %s flag with this existing database, as it has already been initialized to only store "+
				"execution payload headers (aka blinded beacon blocks). If you want to use this flag, you must re-sync your node with a fresh "+
				"database. We recommend using checkpoint sync https://docs.prylabs.network/docs/prysm-usage/checkpoint-sync/",
			features.SaveFullExecutionPayloads.Name,
		)
	}
	if saveFull {
		log.Warn("Saving full beacon blocks to the database. For greater disk space savings, we recommend resyncing from an empty database with " +
			"checkpoint sync to save only blinded beacon blocks by default")
	}
	return nil
}

func createBuckets(tx *bolt.Tx, buckets ...[]byte) error {
	for _, bucket := range buckets {
		if _, err := tx.CreateBucketIfNotExists(bucket); err != nil {
			return err
		}
	}
	return nil
}

// createBoltCollector returns a prometheus collector specifically configured for boltdb.
func createBoltCollector(db *bolt.DB) prometheus.Collector {
	return prombolt.New("boltDB", db, blockedBuckets...)
}
