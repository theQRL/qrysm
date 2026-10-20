package blockchain

import (
	"runtime"
	"sync"
	"testing"

	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/testing/require"
)

func TestService_FinalizedSlotReadConcurrentWithTick(t *testing.T) {
	f := newBatchExecutionFixture(t, 3)
	// Gossip pre-state reads run before the insertion lock, concurrently with
	// the slot ticker's checkpoint updates. Repeated ticks also exercise retries.
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 25; i++ {
			if _, err := f.s.getBlockPreState(f.ctx, f.blks[2].Block()); err != nil {
				errs <- err
				return
			}
			runtime.Gosched()
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 25; i++ {
			if err := f.s.NewSlot(f.ctx, params.BeaconConfig().SlotsPerEpoch); err != nil {
				errs <- err
				return
			}
			runtime.Gosched()
		}
	}()
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
}
