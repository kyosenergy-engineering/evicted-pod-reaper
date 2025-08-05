package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// PodMetrics holds the prometheus metrics for pod operations
type PodMetrics struct {
	deletedTotal *prometheus.CounterVec
	skippedTotal *prometheus.CounterVec
}

// NewPodMetrics creates a new PodMetrics instance
func NewPodMetrics() *PodMetrics {
	return &PodMetrics{
		deletedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "evicted_pods_deleted_total",
				Help: "Total number of evicted pods deleted",
			},
			[]string{"namespace"},
		),
		skippedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "evicted_pods_skipped_total",
				Help: "Total number of evicted pods skipped due to preserve annotation",
			},
			[]string{"namespace"},
		),
	}
}

// Register registers the metrics with the prometheus registry
func (m *PodMetrics) Register(registry prometheus.Registerer) {
	registry.MustRegister(m.deletedTotal)
	registry.MustRegister(m.skippedTotal)
}

// IncDeleted increments the deleted counter for a namespace
func (m *PodMetrics) IncDeleted(namespace string) {
	m.deletedTotal.WithLabelValues(namespace).Inc()
}

// IncSkipped increments the skipped counter for a namespace
func (m *PodMetrics) IncSkipped(namespace string) {
	m.skippedTotal.WithLabelValues(namespace).Inc()
}
