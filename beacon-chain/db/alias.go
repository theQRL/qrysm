package db

import "github.com/theQRL/qrysm/v4/beacon-chain/db/iface"

// ReadOnlyDatabase exposes Qrysm's Zond data backend for read access only, no information about
// head info. For head info, use github.com/theQRL/qrysm/blockchain.HeadFetcher.
type ReadOnlyDatabase = iface.ReadOnlyDatabase

// NoHeadAccessDatabase exposes Qrysm's Zond data backend for read/write access, no information
// about head info. For head info, use github.com/theQRL/qrysm/blockchain.HeadFetcher.
type NoHeadAccessDatabase = iface.NoHeadAccessDatabase

// HeadAccessDatabase exposes Qrysm's Zond backend for read/write access with information about
// chain head information. This interface should be used sparingly as the HeadFetcher is the source
// of truth around chain head information while this interface serves as persistent storage for the
// head fetcher.
//
// See github.com/theQRL/qrysm/blockchain.HeadFetcher
type HeadAccessDatabase = iface.HeadAccessDatabase

// Database defines the necessary methods for Qrysm's Zond backend which may be implemented by any
// key-value or relational database in practice. This is the full database interface which should
// not be used often. Prefer a more restrictive interface in this package.
type Database = iface.Database

// SlasherDatabase defines necessary methods for Qrysm's slasher implementation.
type SlasherDatabase = iface.SlasherDatabase

// ErrExistingGenesisState is an error when the user attempts to save a different genesis state
// when one already exists in a database.
var ErrExistingGenesisState = iface.ErrExistingGenesisState
