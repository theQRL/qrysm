package version

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var qrysmInfo = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "qrysm_version",
	ConstLabels: prometheus.Labels{
		"version":   gitTag,
		"commit":    gitCommit,
		"buildDate": buildDateUnix},
})

func init() {
	qrysmInfo.Set(float64(1))
}
