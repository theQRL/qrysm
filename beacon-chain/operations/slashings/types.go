package slashings

import (
	"context"
	"sync"

	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

// PoolInserter is capable of inserting new slashing objects into the operations pool.
type PoolInserter interface {
	InsertAttesterSlashing(
		ctx context.Context,
		state state.ReadOnlyBeaconState,
		slashing *qrysmpb.AttesterSlashing,
	) error
	InsertProposerSlashing(
		ctx context.Context,
		state state.ReadOnlyBeaconState,
		slashing *qrysmpb.ProposerSlashing,
	) error
}

// PoolManager maintains a pool of pending and recently included attester and proposer slashings.
// This pool is used by proposers to insert data into new blocks.
type PoolManager interface {
	PoolInserter
	PendingAttesterSlashings(ctx context.Context, state state.ReadOnlyBeaconState, noLimit bool) []*qrysmpb.AttesterSlashing
	PendingProposerSlashings(ctx context.Context, state state.ReadOnlyBeaconState, noLimit bool) []*qrysmpb.ProposerSlashing
	MarkIncludedAttesterSlashing(as *qrysmpb.AttesterSlashing)
	MarkIncludedProposerSlashing(ps *qrysmpb.ProposerSlashing)
}

// Pool is a concrete implementation of PoolManager.
type Pool struct {
	lock                    sync.RWMutex
	pendingProposerSlashing []*qrysmpb.ProposerSlashing
	pendingAttesterSlashing []*PendingAttesterSlashing
	included                map[primitives.ValidatorIndex]bool
}

// PendingAttesterSlashing represents an attester slashing in the operation pool.
// Allows for easy binary searching of included validator indexes.
type PendingAttesterSlashing struct {
	attesterSlashing *qrysmpb.AttesterSlashing
	validatorToSlash primitives.ValidatorIndex
}
