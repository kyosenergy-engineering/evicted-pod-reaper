package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestNewPodMetrics(t *testing.T) {
	metrics := NewPodMetrics()

	if metrics == nil {
		t.Fatal("NewPodMetrics() returned nil")
	}

	if metrics.deletedTotal == nil {
		t.Error("deletedTotal counter is nil")
	}

	if metrics.skippedTotal == nil {
		t.Error("skippedTotal counter is nil")
	}
}

func TestPodMetrics_Register(t *testing.T) {
	metrics := NewPodMetrics()
	registry := prometheus.NewRegistry()

	// Should not panic
	metrics.Register(registry)

	// Initialize the metrics with a value to ensure they appear in the registry
	metrics.IncDeleted("test")
	metrics.IncSkipped("test")

	// Verify metrics are registered
	mfs, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	metricNames := make(map[string]bool)
	for _, mf := range mfs {
		metricNames[mf.GetName()] = true
	}

	if !metricNames["evicted_pods_deleted_total"] {
		t.Error("evicted_pods_deleted_total metric not registered")
	}

	if !metricNames["evicted_pods_skipped_total"] {
		t.Error("evicted_pods_skipped_total metric not registered")
	}
}

func TestPodMetrics_IncDeleted(t *testing.T) {
	metrics := NewPodMetrics()
	registry := prometheus.NewRegistry()
	metrics.Register(registry)

	tests := []struct {
		name      string
		namespace string
		want      float64
	}{
		{
			name:      "increment default namespace",
			namespace: "default",
			want:      1,
		},
		{
			name:      "increment kube-system namespace",
			namespace: "kube-system",
			want:      1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset the metric for this test
			metrics.deletedTotal.Reset()

			// Increment the counter
			metrics.IncDeleted(tt.namespace)

			// Verify the counter value
			count := testutil.ToFloat64(metrics.deletedTotal.WithLabelValues(tt.namespace))
			if count != tt.want {
				t.Errorf("IncDeleted() counter = %v, want %v", count, tt.want)
			}
		})
	}
}

func TestPodMetrics_IncSkipped(t *testing.T) {
	metrics := NewPodMetrics()
	registry := prometheus.NewRegistry()
	metrics.Register(registry)

	tests := []struct {
		name      string
		namespace string
		want      float64
	}{
		{
			name:      "increment default namespace",
			namespace: "default",
			want:      1,
		},
		{
			name:      "increment monitoring namespace",
			namespace: "monitoring",
			want:      1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset the metric for this test
			metrics.skippedTotal.Reset()

			// Increment the counter
			metrics.IncSkipped(tt.namespace)

			// Verify the counter value
			count := testutil.ToFloat64(metrics.skippedTotal.WithLabelValues(tt.namespace))
			if count != tt.want {
				t.Errorf("IncSkipped() counter = %v, want %v", count, tt.want)
			}
		})
	}
}

func TestPodMetrics_MultipleIncrements(t *testing.T) {
	metrics := NewPodMetrics()
	registry := prometheus.NewRegistry()
	metrics.Register(registry)

	// Reset metrics
	metrics.deletedTotal.Reset()
	metrics.skippedTotal.Reset()

	// Increment deleted counter multiple times for same namespace
	metrics.IncDeleted("default")
	metrics.IncDeleted("default")
	metrics.IncDeleted("default")

	// Increment skipped counter multiple times for different namespaces
	metrics.IncSkipped("default")
	metrics.IncSkipped("kube-system")
	metrics.IncSkipped("kube-system")

	// Verify deleted counter
	deletedCount := testutil.ToFloat64(metrics.deletedTotal.WithLabelValues("default"))
	if deletedCount != 3 {
		t.Errorf("IncDeleted() multiple calls: got %v, want 3", deletedCount)
	}

	// Verify skipped counters
	skippedDefault := testutil.ToFloat64(metrics.skippedTotal.WithLabelValues("default"))
	if skippedDefault != 1 {
		t.Errorf("IncSkipped() default namespace: got %v, want 1", skippedDefault)
	}

	skippedKubeSystem := testutil.ToFloat64(metrics.skippedTotal.WithLabelValues("kube-system"))
	if skippedKubeSystem != 2 {
		t.Errorf("IncSkipped() kube-system namespace: got %v, want 2", skippedKubeSystem)
	}
}

func TestPodMetrics_MetricLabels(t *testing.T) {
	metrics := NewPodMetrics()
	registry := prometheus.NewRegistry()
	metrics.Register(registry)

	// Increment counters with specific namespaces
	metrics.IncDeleted("test-namespace")
	metrics.IncSkipped("another-namespace")

	// Gather metrics
	mfs, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	// Check that metrics have the correct labels
	for _, mf := range mfs {
		if mf.GetName() == "evicted_pods_deleted_total" {
			for _, m := range mf.GetMetric() {
				labels := m.GetLabel()
				if len(labels) != 1 {
					t.Errorf("Expected 1 label, got %d", len(labels))
				}
				if labels[0].GetName() != "namespace" {
					t.Errorf("Expected label name 'namespace', got '%s'", labels[0].GetName())
				}
				if labels[0].GetValue() != "test-namespace" {
					t.Errorf("Expected label value 'test-namespace', got '%s'", labels[0].GetValue())
				}
			}
		}

		if mf.GetName() == "evicted_pods_skipped_total" {
			for _, m := range mf.GetMetric() {
				labels := m.GetLabel()
				if len(labels) != 1 {
					t.Errorf("Expected 1 label, got %d", len(labels))
				}
				if labels[0].GetName() != "namespace" {
					t.Errorf("Expected label name 'namespace', got '%s'", labels[0].GetName())
				}
				if labels[0].GetValue() != "another-namespace" {
					t.Errorf("Expected label value 'another-namespace', got '%s'", labels[0].GetValue())
				}
			}
		}
	}
}
