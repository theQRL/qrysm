package blstoexec

import (
	"math"
	"sync"

	"github.com/cyyber/qrysm/v4/beacon-chain/core/blocks"
	"github.com/cyyber/qrysm/v4/beacon-chain/state"
	"github.com/cyyber/qrysm/v4/config/params"
	"github.com/cyyber/qrysm/v4/consensus-types/primitives"
	doublylinkedlist "github.com/cyyber/qrysm/v4/container/doubly-linked-list"
	ethpb "github.com/cyyber/qrysm/v4/proto/prysm/v1alpha1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sirupsen/logrus"
)

// We recycle the Dilithium changes pool to avoid the backing map growing without
// bound. The cycling operation is expensive because it copies all elements, so
// we only do it when the map is smaller than this upper bound.
const dilithiumChangesPoolThreshold = 2000

var (
	dilithiumToExecMessageInPoolTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "dilithium_to_exec_message_pool_total",
		Help: "The number of saved dilithium to exec messages in the operation pool.",
	})
)

// PoolManager maintains pending and seen Dilithium-to-execution-change objects.
// This pool is used by proposers to insert Dilithium-to-execution-change objects into new blocks.
type PoolManager interface {
	PendingDilithiumToExecChanges() ([]*ethpb.SignedDilithiumToExecutionChange, error)
	DilithiumToExecChangesForInclusion(beaconState state.ReadOnlyBeaconState) ([]*ethpb.SignedDilithiumToExecutionChange, error)
	InsertDilithiumToExecChange(change *ethpb.SignedDilithiumToExecutionChange)
	MarkIncluded(change *ethpb.SignedDilithiumToExecutionChange)
	ValidatorExists(idx primitives.ValidatorIndex) bool
}

// Pool is a concrete implementation of PoolManager.
type Pool struct {
	lock    sync.RWMutex
	pending doublylinkedlist.List[*ethpb.SignedDilithiumToExecutionChange]
	m       map[primitives.ValidatorIndex]*doublylinkedlist.Node[*ethpb.SignedDilithiumToExecutionChange]
}

// NewPool returns an initialized pool.
func NewPool() *Pool {
	return &Pool{
		pending: doublylinkedlist.List[*ethpb.SignedDilithiumToExecutionChange]{},
		m:       make(map[primitives.ValidatorIndex]*doublylinkedlist.Node[*ethpb.SignedDilithiumToExecutionChange]),
	}
}

// Copies the internal map and returns a new one.
func (p *Pool) cycleMap() {
	newMap := make(map[primitives.ValidatorIndex]*doublylinkedlist.Node[*ethpb.SignedDilithiumToExecutionChange])
	for k, v := range p.m {
		newMap[k] = v
	}
	p.m = newMap
}

// PendingDilithiumToExecChanges returns all objects from the pool.
func (p *Pool) PendingDilithiumToExecChanges() ([]*ethpb.SignedDilithiumToExecutionChange, error) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	result := make([]*ethpb.SignedDilithiumToExecutionChange, p.pending.Len())
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

// DilithiumToExecChangesForInclusion returns objects that are ready for inclusion.
// This method will not return more than the block enforced MaxDilithiumToExecutionChanges.
func (p *Pool) DilithiumToExecChangesForInclusion(st state.ReadOnlyBeaconState) ([]*ethpb.SignedDilithiumToExecutionChange, error) {
	p.lock.RLock()
	defer p.lock.RUnlock()
	length := int(math.Min(float64(params.BeaconConfig().MaxDilithiumToExecutionChanges), float64(p.pending.Len())))
	result := make([]*ethpb.SignedDilithiumToExecutionChange, 0, length)
	node := p.pending.Last()
	for node != nil && len(result) < length {
		change, err := node.Value()
		if err != nil {
			return nil, err
		}
		_, err = blocks.ValidateDilithiumToExecutionChange(st, change)
		if err != nil {
			logrus.WithError(err).Warning("removing invalid DilithiumToExecutionChange from pool")
			// MarkIncluded removes the invalid change from the pool
			p.lock.RUnlock()
			p.MarkIncluded(change)
			p.lock.RLock()
		} else {
			result = append(result, change)
		}
		node, err = node.Prev()
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

// InsertDilithiumToExecChange inserts an object into the pool.
func (p *Pool) InsertDilithiumToExecChange(change *ethpb.SignedDilithiumToExecutionChange) {
	p.lock.Lock()
	defer p.lock.Unlock()

	_, exists := p.m[change.Message.ValidatorIndex]
	if exists {
		return
	}

	p.pending.Append(doublylinkedlist.NewNode(change))
	p.m[change.Message.ValidatorIndex] = p.pending.Last()

	dilithiumToExecMessageInPoolTotal.Inc()
}

// MarkIncluded is used when an object has been included in a beacon block. Every block seen by this
// node should call this method to include the object. This will remove the object from the pool.
func (p *Pool) MarkIncluded(change *ethpb.SignedDilithiumToExecutionChange) {
	p.lock.Lock()
	defer p.lock.Unlock()

	node := p.m[change.Message.ValidatorIndex]
	if node == nil {
		return
	}

	delete(p.m, change.Message.ValidatorIndex)
	p.pending.Remove(node)
	if p.numPending() == dilithiumChangesPoolThreshold {
		p.cycleMap()
	}

	dilithiumToExecMessageInPoolTotal.Dec()
}

// ValidatorExists checks if the dilithium to execution change object exists
// for that particular validator.
func (p *Pool) ValidatorExists(idx primitives.ValidatorIndex) bool {
	p.lock.RLock()
	defer p.lock.RUnlock()

	node := p.m[idx]

	return node != nil
}

// numPending returns the number of pending dilithium to execution changes in the pool
func (p *Pool) numPending() int {
	return p.pending.Len()
}
