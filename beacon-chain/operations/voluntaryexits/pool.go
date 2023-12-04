package voluntaryexits

import (
	"math"
	"sync"

	"github.com/sirupsen/logrus"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/v4/beacon-chain/state"
	"github.com/theQRL/qrysm/v4/config/params"
	types "github.com/theQRL/qrysm/v4/consensus-types/primitives"
	doublylinkedlist "github.com/theQRL/qrysm/v4/container/doubly-linked-list"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/time/slots"
)

// PoolManager maintains pending and seen voluntary exits.
// This pool is used by proposers to insert voluntary exits into new blocks.
type PoolManager interface {
	PendingExits() ([]*zondpb.SignedVoluntaryExit, error)
	ExitsForInclusion(state state.ReadOnlyBeaconState, slot types.Slot) ([]*zondpb.SignedVoluntaryExit, error)
	InsertVoluntaryExit(exit *zondpb.SignedVoluntaryExit)
	MarkIncluded(exit *zondpb.SignedVoluntaryExit)
}

// Pool is a concrete implementation of PoolManager.
type Pool struct {
	lock    sync.RWMutex
	pending doublylinkedlist.List[*zondpb.SignedVoluntaryExit]
	m       map[types.ValidatorIndex]*doublylinkedlist.Node[*zondpb.SignedVoluntaryExit]
}

// NewPool returns an initialized pool.
func NewPool() *Pool {
	return &Pool{
		pending: doublylinkedlist.List[*zondpb.SignedVoluntaryExit]{},
		m:       make(map[types.ValidatorIndex]*doublylinkedlist.Node[*zondpb.SignedVoluntaryExit]),
	}
}

// PendingExits returns all objects from the pool.
func (p *Pool) PendingExits() ([]*zondpb.SignedVoluntaryExit, error) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	result := make([]*zondpb.SignedVoluntaryExit, p.pending.Len())
	node := p.pending.First()
	var err error
	for i := 0; node != nil; i++ {
		result[i], err = node.Value()
		if err != nil {
			return nil, err
		}
		node, err = node.Next()
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

// ExitsForInclusion returns objects that are ready for inclusion at the given slot. This method will not
// return more than the block enforced MaxVoluntaryExits.
func (p *Pool) ExitsForInclusion(state state.ReadOnlyBeaconState, slot types.Slot) ([]*zondpb.SignedVoluntaryExit, error) {
	p.lock.RLock()
	length := int(math.Min(float64(params.BeaconConfig().MaxVoluntaryExits), float64(p.pending.Len())))
	result := make([]*zondpb.SignedVoluntaryExit, 0, length)
	node := p.pending.First()
	for node != nil && len(result) < length {
		exit, err := node.Value()
		if err != nil {
			p.lock.RUnlock()
			return nil, err
		}
		if exit.Exit.Epoch > slots.ToEpoch(slot) {
			node, err = node.Next()
			if err != nil {
				p.lock.RUnlock()
				return nil, err
			}
			continue
		}
		validator, err := state.ValidatorAtIndexReadOnly(exit.Exit.ValidatorIndex)
		if err != nil {
			logrus.WithError(err).Warningf("could not get validator at index %d", exit.Exit.ValidatorIndex)
			node, err = node.Next()
			if err != nil {
				p.lock.RUnlock()
				return nil, err
			}
			continue
		}
		if err = blocks.VerifyExitAndSignature(validator, state, exit); err != nil {
			logrus.WithError(err).Warning("removing invalid exit from pool")
			p.lock.RUnlock()
			// MarkIncluded removes the invalid exit from the pool
			p.MarkIncluded(exit)
			p.lock.RLock()
		} else {
			result = append(result, exit)
		}
		node, err = node.Next()
		if err != nil {
			p.lock.RUnlock()
			return nil, err
		}
	}
	p.lock.RUnlock()
	return result, nil
}

// InsertVoluntaryExit into the pool.
func (p *Pool) InsertVoluntaryExit(exit *zondpb.SignedVoluntaryExit) {
	p.lock.Lock()
	defer p.lock.Unlock()

	_, exists := p.m[exit.Exit.ValidatorIndex]
	if exists {
		return
	}

	p.pending.Append(doublylinkedlist.NewNode(exit))
	p.m[exit.Exit.ValidatorIndex] = p.pending.Last()
}

// MarkIncluded is used when an exit has been included in a beacon block. Every block seen by this
// node should call this method to include the exit. This will remove the exit from the pool.
func (p *Pool) MarkIncluded(exit *zondpb.SignedVoluntaryExit) {
	p.lock.Lock()
	defer p.lock.Unlock()

	node := p.m[exit.Exit.ValidatorIndex]
	if node == nil {
		return
	}

	delete(p.m, exit.Exit.ValidatorIndex)
	p.pending.Remove(node)
}
