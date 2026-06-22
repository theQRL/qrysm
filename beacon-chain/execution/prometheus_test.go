package execution

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/theQRL/qrysm/testing/assert"
)

// TestCleanup ensures that the cleanup function unregisters the prometheus.Collection
// also tests the interchangability of the explicit prometheus Register/Unregister
// and the implicit methods within the collector implementation
func TestCleanup(t *testing.T) {
	ctx := context.Background()
	pc, err := NewExecutionChainCollector(ctx)
	assert.NoError(t, err, "Unexpected error calling NewExecutionChainCollector")
	unregistered := pc.unregister()
	assert.Equal(t, true, unregistered, "ExecutionChainCollector.unregister did not return true (via prometheus.DefaultRegistry)")
	// ExecutionChainCollector is a prometheus.Collector, so we should be able to register it again
	err = prometheus.Register(pc)
	assert.NoError(t, err, "Got error from prometheus.Register after unregistering ExecutionChainCollector")
	// even if it somehow gets registered somewhere else, unregister should work
	unregistered = pc.unregister()
	assert.Equal(t, true, unregistered, "ExecutionChainCollector.unregister failed on the second attempt")
	// and so we should be able to register it again
	err = prometheus.Register(pc)
	assert.NoError(t, err, "Got error from prometheus.Register on the second attempt")
	// ok clean it up one last time for real :)
	unregistered = prometheus.Unregister(pc)
	assert.Equal(t, true, unregistered, "prometheus.Unregister failed to unregister ExecutionChainCollector on final cleanup")
}

// TestCancelation tests that canceling the context passed into
// NewExecutionChainCollector cleans everything up as expected. This
// does come at the cost of an extra channel cluttering up
// ExecutionChainCollector, just for this test.
func TestCancelation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	pc, err := NewExecutionChainCollector(ctx)
	assert.NoError(t, err, "Unexpected error calling NewExecutionChainCollector")
	ticker := time.NewTicker(10 * time.Second)
	cancel()
	select {
	case <-ticker.C:
		t.Error("Hit timeout waiting for cancel() to cleanup ExecutionChainCollector")
	case <-pc.finishChan:
		break
	}
	err = prometheus.Register(pc)
	assert.NoError(t, err, "Got error from prometheus.Register after unregistering ExecutionChainCollector through canceled context")
}
