package metric

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"time"
)

var (
	// SLBLatency reconcile SLB latency
	SLBLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "ccm_slb_latencies_duration_milliseconds",
			Help: "CCM load balancer reconcile latency distribution in milliseconds for each verb.",
			Buckets: []float64{100, 200, 300, 400, 500, 600, 700, 800, 900, 1000,
				1500, 2000, 3000, 4000, 5000, 6000, 7000, 8000, 9000, 10000},
		},
		[]string{"verb"},
	)
)

// MsSince returns milliseconds since start.
func MsSince(start time.Time) float64 {
	return float64(time.Since(start) / time.Millisecond)
}

// RegisterPrometheus register metrics to prometheus server
func RegisterPrometheus() {
	metrics.Registry.MustRegister(SLBLatency)
}
