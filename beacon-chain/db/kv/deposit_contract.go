package kv

import (
	"context"
	"fmt"

	"github.com/theQRL/go-qrl/common"
	bolt "go.etcd.io/bbolt"
	"go.opencensus.io/trace"
)

// DepositContractAddress returns contract address is the address of
// the deposit contract on the execution chain.
func (s *Store) DepositContractAddress(ctx context.Context) ([]byte, error) {
	_, span := trace.StartSpan(ctx, "BeaconDB.DepositContractAddress")
	defer span.End()
	var addr []byte
	if err := s.db.View(func(tx *bolt.Tx) error {
		chainInfo := tx.Bucket(chainMetadataBucket)
		v := chainInfo.Get(depositContractAddressKey)
		if len(v) > 0 {
			addr = make([]byte, len(v))
			copy(addr, v)
		}
		return nil
	}); err != nil { // This view never returns an error, but we'll handle anyway for sanity.
		panic(err) // lint:nopanic
	}
	return addr, nil
}

// SaveDepositContractAddress to the db. It returns an error if an address has been previously saved.
func (s *Store) SaveDepositContractAddress(ctx context.Context, addr common.Address) error {
	_, span := trace.StartSpan(ctx, "BeaconDB.VerifyContractAddress")
	defer span.End()

	return s.db.Update(func(tx *bolt.Tx) error {
		chainInfo := tx.Bucket(chainMetadataBucket)
		expectedAddress := chainInfo.Get(depositContractAddressKey)
		if expectedAddress != nil {
			return fmt.Errorf("cannot override deposit contract address: %v", expectedAddress)
		}
		return chainInfo.Put(depositContractAddressKey, addr.Bytes())
	})
}
