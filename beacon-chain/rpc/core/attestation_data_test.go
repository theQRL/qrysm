package core

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	chainmock "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	"github.com/theQRL/qrysm/beacon-chain/cache"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

type attestationReorgHead struct {
	*chainmock.ChainService
	mu              sync.Mutex
	root            [32]byte
	st              state.BeaconState
	optimisticRoots map[[32]byte]bool
	afterSnapshot   func()
	afterOptimistic func()
}

func (h *attestationReorgHead) HeadStateAndRoot(context.Context) (state.BeaconState, []byte, error) {
	h.mu.Lock()
	st, root := h.st.Copy(), h.root
	h.mu.Unlock()
	if h.afterSnapshot != nil {
		h.afterSnapshot()
	}
	return st, root[:], nil
}

// Separate reads deliberately describe different branches. The RPC must use
// the atomic snapshot, rather than combine these otherwise valid responses.
func (h *attestationReorgHead) HeadState(context.Context) (state.BeaconState, error) {
	st := h.ChainService.State.Copy()
	if h.afterSnapshot != nil {
		h.afterSnapshot()
	}
	return st, nil
}

func (h *attestationReorgHead) HeadRoot(context.Context) ([]byte, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return bytesutil.SafeCopyBytes(h.root[:]), nil
}

func (h *attestationReorgHead) IsOptimistic(context.Context) (bool, error) {
	h.mu.Lock()
	optimistic := h.optimisticRoots[h.root]
	h.mu.Unlock()
	if h.afterOptimistic != nil {
		h.afterOptimistic()
	}
	return optimistic, nil
}

func (h *attestationReorgHead) IsOptimisticForRoot(_ context.Context, root [32]byte) (bool, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.optimisticRoots[root], nil
}

func attestationReorgState(t *testing.T, slot primitives.Slot, target [32]byte) state.BeaconState {
	t.Helper()
	st, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, st.SetSlot(slot))
	rootSlots := st.BlockRoots()
	rootSlots[params.BeaconConfig().SlotsPerEpoch] = target[:]
	require.NoError(t, st.SetBlockRoots(rootSlots))
	return st
}

func TestGetAttestationData_HeadSnapshot(t *testing.T) {
	e := params.BeaconConfig().SlotsPerEpoch
	oldTarget, newTarget, newRoot := [32]byte{'o'}, [32]byte{'n'}, [32]byte{'h'}
	h := &attestationReorgHead{
		ChainService: &chainmock.ChainService{State: attestationReorgState(t, e+1, oldTarget)},
		root:         newRoot, st: attestationReorgState(t, e+2, newTarget),
	}
	s := &Service{
		HeadFetcher: h, OptimisticModeFetcher: h, AttestationCache: cache.NewAttestationCache(),
		GenesisTimeFetcher: &chainmock.ChainService{Genesis: time.Now().Add(-time.Duration(e+2) * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second)},
	}
	res, rpcErr := s.GetAttestationData(context.Background(), &qrysmpb.AttestationDataRequest{Slot: e + 2})
	require.Equal(t, (*RpcError)(nil), rpcErr)
	assert.Equal(t, newRoot, bytesutil.ToBytes32(res.BeaconBlockRoot))
	assert.Equal(t, newTarget, bytesutil.ToBytes32(res.Target.Root))
}

func TestGetAttestationData_ReorgDuringRequest(t *testing.T) {
	for _, pauseAt := range []string{"optimistic check", "head snapshot"} {
		for _, optimistic := range []bool{false, true} {
			t.Run(pauseAt+map[bool]string{false: "/validated replacement", true: "/optimistic replacement"}[optimistic], func(t *testing.T) {
				e := params.BeaconConfig().SlotsPerEpoch
				oldRoot, newRoot := [32]byte{'o'}, [32]byte{'n'}
				oldTarget, newTarget := [32]byte{'a'}, [32]byte{'b'}
				oldState := attestationReorgState(t, e+1, oldTarget)
				newState := attestationReorgState(t, e+2, newTarget)
				synctest.Test(t, func(t *testing.T) {
					entered, release := make(chan struct{}), make(chan struct{})
					resume := sync.OnceFunc(func() { close(release) })
					defer resume()
					var paused atomic.Bool
					pause := func() {
						if paused.CompareAndSwap(false, true) {
							close(entered)
							<-release
						}
					}
					h := &attestationReorgHead{
						ChainService: &chainmock.ChainService{State: oldState}, root: oldRoot, st: oldState,
						optimisticRoots: map[[32]byte]bool{newRoot: optimistic},
					}
					if pauseAt == "optimistic check" {
						h.afterOptimistic = pause
					} else {
						h.afterSnapshot = pause
					}
					c := cache.NewAttestationCache()
					s := &Service{
						HeadFetcher: h, OptimisticModeFetcher: h, AttestationCache: c,
						GenesisTimeFetcher: &chainmock.ChainService{Genesis: time.Now().Add(-time.Duration(e+2) * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second)},
					}
					type result struct {
						data *qrysmpb.AttestationData
						err  *RpcError
					}
					request := func(index primitives.CommitteeIndex) result {
						data, err := s.GetAttestationData(context.Background(), &qrysmpb.AttestationDataRequest{Slot: e + 2, CommitteeIndex: index})
						return result{data, err}
					}
					first, waiter := make(chan result, 1), make(chan result, 1)
					go func() { first <- request(0) }()
					<-entered
					go func() { waiter <- request(1) }()
					synctest.Wait()
					// Match publication order: invalidate producers before making
					// the replacement snapshot visible under the head lock.
					h.mu.Lock()
					c.Clear()
					h.root, h.st = newRoot, newState
					h.mu.Unlock()
					resume()
					results := []result{<-first, <-waiter, request(2)}
					for i, got := range results {
						if optimistic {
							require.NotNil(t, got.err)
							assert.Equal(t, ErrorReason(Unavailable), got.err.Reason)
							require.ErrorIs(t, got.err.Err, errOptimisticMode)
							continue
						}
						require.Equal(t, (*RpcError)(nil), got.err)
						assert.Equal(t, primitives.CommitteeIndex(i), got.data.CommitteeIndex)
						assert.Equal(t, newRoot, bytesutil.ToBytes32(got.data.BeaconBlockRoot))
						assert.Equal(t, newTarget, bytesutil.ToBytes32(got.data.Target.Root))
					}
				})
			})
		}
	}
}
