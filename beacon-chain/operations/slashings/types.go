package slashings

import (
	"context"
	"sync"

	"github.com/theQRL/qrysm/v4/beacon-chain/state"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
)

// PoolInserter is capable of inserting new slashing objects into the operations pool.
type PoolInserter interface {
	InsertAttesterSlashing(
		ctx context.Context,
		state state.ReadOnlyBeaconState,
		slashing *zondpb.AttesterSlashing,
	) error
	InsertProposerSlashing(
		ctx context.Context,
		state state.ReadOnlyBeaconState,
		slashing *zondpb.ProposerSlashing,
	) error
}

// PoolManager maintains a pool of pending and recently included attester and proposer slashings.
// This pool is used by proposers to insert data into new blocks.
type PoolManager interface {
	PoolInserter
	PendingAttesterSlashings(ctx context.Context, state state.ReadOnlyBeaconState, noLimit bool) []*zondpb.AttesterSlashing
	PendingProposerSlashings(ctx context.Context, state state.ReadOnlyBeaconState, noLimit bool) []*zondpb.ProposerSlashing
	MarkIncludedAttesterSlashing(as *zondpb.AttesterSlashing)
	MarkIncludedProposerSlashing(ps *zondpb.ProposerSlashing)
}

// Pool is a concrete implementation of PoolManager.
type Pool struct {
	lock                    sync.RWMutex
	pendingProposerSlashing []*zondpb.ProposerSlashing
	pendingAttesterSlashing []*PendingAttesterSlashing
	included                map[primitives.ValidatorIndex]bool
}

// PendingAttesterSlashing represents an attester slashing in the operation pool.
// Allows for easy binary searching of included validator indexes.
type PendingAttesterSlashing struct {
	attesterSlashing *zondpb.AttesterSlashing
	validatorToSlash primitives.ValidatorIndex
}
