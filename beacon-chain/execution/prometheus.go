package execution

import (
	"context"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/theQRL/qrysm/monitoring/clientstats"
)

type BeaconNodeStatsUpdater interface {
	Update(stats clientstats.BeaconNodeStats)
}

type ExecutionChainCollector struct {
	SyncExecutionConnected *prometheus.Desc
	updateChan             chan clientstats.BeaconNodeStats
	latestStats            clientstats.BeaconNodeStats
	sync.Mutex
	ctx        context.Context
	finishChan chan struct{}
}

var _ BeaconNodeStatsUpdater = &ExecutionChainCollector{}
var _ prometheus.Collector = &ExecutionChainCollector{}

// Update satisfies the BeaconNodeStatsUpdater
func (pc *ExecutionChainCollector) Update(update clientstats.BeaconNodeStats) {
	pc.updateChan <- update
}

// Describe is invoked by the prometheus collection loop.
// It returns a set of metric Descriptor references which
// are also used in Collect to group collected metrics into
// a family. Describe and Collect together satisfy the
// prometheus.Collector interface.
func (pc *ExecutionChainCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- pc.SyncExecutionConnected
}

// Collect is invoked by the prometheus collection loop.
// It returns a set of Metrics representing the observation
// for the current collection period. In the case of this
// collector, we use values from the latest BeaconNodeStats
// value sent by the execution chain Service, which updates this value
// whenever an internal event could change the state of one of
// the metrics.
// Describe and Collect together satisfy the
// prometheus.Collector interface.
func (pc *ExecutionChainCollector) Collect(ch chan<- prometheus.Metric) {
	bs := pc.getLatestStats()

	var syncExecutionConnected float64 = 0
	if bs.SyncExecutionConnected {
		syncExecutionConnected = 1
	}
	ch <- prometheus.MustNewConstMetric(
		pc.SyncExecutionConnected,
		prometheus.GaugeValue,
		syncExecutionConnected,
	)
}

func (pc *ExecutionChainCollector) getLatestStats() clientstats.BeaconNodeStats {
	pc.Lock()
	defer pc.Unlock()
	return pc.latestStats
}

func (pc *ExecutionChainCollector) setLatestStats(bs clientstats.BeaconNodeStats) {
	pc.Lock()
	pc.latestStats = bs
	pc.Unlock()
}

// unregister returns true if the prometheus DefaultRegistry
// confirms that it was removed.
func (pc *ExecutionChainCollector) unregister() bool {
	return prometheus.Unregister(pc)
}

func (pc *ExecutionChainCollector) latestStatsUpdateLoop() {
	for {
		select {
		case <-pc.ctx.Done():
			pc.unregister()
			pc.finishChan <- struct{}{}
			return
		case bs := <-pc.updateChan:
			pc.setLatestStats(bs)
		}
	}
}

func NewExecutionChainCollector(ctx context.Context) (*ExecutionChainCollector, error) {
	namespace := "execution_chain"
	updateChan := make(chan clientstats.BeaconNodeStats, 2)
	c := &ExecutionChainCollector{
		SyncExecutionConnected: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "sync_execution_connected"),
			"Boolean indicating whether an execution endpoint is currently connected: 0=false, 1=true.",
			nil,
			nil,
		),
		updateChan: updateChan,
		ctx:        ctx,
		finishChan: make(chan struct{}, 1),
	}
	go c.latestStatsUpdateLoop()
	return c, prometheus.Register(c)
}

type NopBeaconNodeStatsUpdater struct{}

func (_ *NopBeaconNodeStatsUpdater) Update(_ clientstats.BeaconNodeStats) {}

var _ BeaconNodeStatsUpdater = &NopBeaconNodeStatsUpdater{}
