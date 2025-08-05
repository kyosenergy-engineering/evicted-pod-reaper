package controller

import (
	"context"
	"testing"
	"time"

	"github.com/kyosenergy/evicted-pod-reaper/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestPodReconciler_ReconcileWithMetrics(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)

	tests := []struct {
		name             string
		pod              *corev1.Pod
		ttl              int
		wantDeletedCount float64
		wantSkippedCount float64
	}{
		{
			name: "deleted pod should increment deleted metric",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod-deleted",
					Namespace: "test-namespace",
				},
				Status: corev1.PodStatus{
					Phase:     corev1.PodFailed,
					Reason:    "Evicted",
					StartTime: &metav1.Time{Time: time.Now().Add(-10 * time.Minute)},
				},
			},
			ttl:              300,
			wantDeletedCount: 1,
			wantSkippedCount: 0,
		},
		{
			name: "preserved pod should increment skipped metric",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod-preserved",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						"pod-reaper.kyos.com/preserve": "true",
					},
				},
				Status: corev1.PodStatus{
					Phase:     corev1.PodFailed,
					Reason:    "Evicted",
					StartTime: &metav1.Time{Time: time.Now().Add(-10 * time.Minute)},
				},
			},
			ttl:              300,
			wantDeletedCount: 0,
			wantSkippedCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new metrics instance and register it
			podMetrics := metrics.NewPodMetrics()
			registry := prometheus.NewRegistry()
			podMetrics.Register(registry)

			// Create fake client with the pod
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(tt.pod).
				Build()

			// Create reconciler with metrics
			r := &PodReconciler{
				Client:      fakeClient,
				Scheme:      scheme,
				Metrics:     podMetrics,
				TTLToDelete: tt.ttl,
			}

			// Run reconcile
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      tt.pod.Name,
					Namespace: tt.pod.Namespace,
				},
			}
			_, err := r.Reconcile(context.Background(), req)
			if err != nil {
				t.Fatalf("Reconcile() error = %v", err)
			}

			// Gather metrics from the registry
			mfs, err := registry.Gather()
			if err != nil {
				t.Fatalf("Failed to gather metrics: %v", err)
			}

			var actualDeletedCount float64
			var actualSkippedCount float64

			for _, mf := range mfs {
				if mf.GetName() == "evicted_pods_deleted_total" {
					for _, m := range mf.GetMetric() {
						for _, label := range m.GetLabel() {
							if label.GetName() == "namespace" && label.GetValue() == tt.pod.Namespace {
								actualDeletedCount = m.GetCounter().GetValue()
							}
						}
					}
				}
				if mf.GetName() == "evicted_pods_skipped_total" {
					for _, m := range mf.GetMetric() {
						for _, label := range m.GetLabel() {
							if label.GetName() == "namespace" && label.GetValue() == tt.pod.Namespace {
								actualSkippedCount = m.GetCounter().GetValue()
							}
						}
					}
				}
			}

			if actualDeletedCount != tt.wantDeletedCount {
				t.Errorf("Deleted metric = %v, want %v", actualDeletedCount, tt.wantDeletedCount)
			}
			if actualSkippedCount != tt.wantSkippedCount {
				t.Errorf("Skipped metric = %v, want %v", actualSkippedCount, tt.wantSkippedCount)
			}
		})
	}
}

func TestPodReconciler_MetricsAcrossMultipleReconciles(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)

	// Create metrics and registry
	podMetrics := metrics.NewPodMetrics()
	registry := prometheus.NewRegistry()
	podMetrics.Register(registry)

	// Create multiple pods
	pods := []*corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod1",
				Namespace: "default",
			},
			Status: corev1.PodStatus{
				Phase:     corev1.PodFailed,
				Reason:    "Evicted",
				StartTime: &metav1.Time{Time: time.Now().Add(-10 * time.Minute)},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod2",
				Namespace: "default",
				Annotations: map[string]string{
					"pod-reaper.kyos.com/preserve": "true",
				},
			},
			Status: corev1.PodStatus{
				Phase:     corev1.PodFailed,
				Reason:    "Evicted",
				StartTime: &metav1.Time{Time: time.Now().Add(-10 * time.Minute)},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod3",
				Namespace: "kube-system",
			},
			Status: corev1.PodStatus{
				Phase:     corev1.PodFailed,
				Reason:    "Evicted",
				StartTime: &metav1.Time{Time: time.Now().Add(-10 * time.Minute)},
			},
		},
	}

	// Create fake client with all pods
	objs := make([]runtime.Object, len(pods))
	for i, pod := range pods {
		objs[i] = pod
	}
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(objs...).
		Build()

	// Create reconciler
	r := &PodReconciler{
		Client:      fakeClient,
		Scheme:      scheme,
		Metrics:     podMetrics,
		TTLToDelete: 300,
	}

	// Reconcile each pod
	for _, pod := range pods {
		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      pod.Name,
				Namespace: pod.Namespace,
			},
		}
		_, err := r.Reconcile(context.Background(), req)
		if err != nil {
			t.Fatalf("Reconcile() error for pod %s: %v", pod.Name, err)
		}
	}

	// Gather and check metrics
	mfs, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	expectedMetrics := map[string]map[string]float64{
		"evicted_pods_deleted_total": {
			"default":     1, // pod1 deleted
			"kube-system": 1, // pod3 deleted
		},
		"evicted_pods_skipped_total": {
			"default": 1, // pod2 skipped
		},
	}

	for _, mf := range mfs {
		metricName := mf.GetName()
		if expected, ok := expectedMetrics[metricName]; ok {
			for _, m := range mf.GetMetric() {
				for _, label := range m.GetLabel() {
					if label.GetName() == "namespace" {
						namespace := label.GetValue()
						value := m.GetCounter().GetValue()

						if expectedValue, exists := expected[namespace]; exists {
							if value != expectedValue {
								t.Errorf("%s{namespace=%q} = %v, want %v",
									metricName, namespace, value, expectedValue)
							}
						} else if value != 0 {
							t.Errorf("%s{namespace=%q} = %v, want 0 (no activity expected)",
								metricName, namespace, value)
						}
					}
				}
			}
		}
	}
}

func TestPodReconciler_NoMetricsForNonEvictedPods(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)

	// Create metrics and registry
	podMetrics := metrics.NewPodMetrics()
	registry := prometheus.NewRegistry()
	podMetrics.Register(registry)

	// Create non-evicted pods
	pods := []*corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "running-pod",
				Namespace: "default",
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "failed-pod-oom",
				Namespace: "default",
			},
			Status: corev1.PodStatus{
				Phase:  corev1.PodFailed,
				Reason: "OOMKilled",
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pending-pod",
				Namespace: "default",
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
			},
		},
	}

	// Create fake client with all pods
	objs := make([]runtime.Object, len(pods))
	for i, pod := range pods {
		objs[i] = pod
	}
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(objs...).
		Build()

	// Create reconciler
	r := &PodReconciler{
		Client:      fakeClient,
		Scheme:      scheme,
		Metrics:     podMetrics,
		TTLToDelete: 300,
	}

	// Reconcile each pod
	for _, pod := range pods {
		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      pod.Name,
				Namespace: pod.Namespace,
			},
		}
		_, err := r.Reconcile(context.Background(), req)
		if err != nil {
			t.Fatalf("Reconcile() error for pod %s: %v", pod.Name, err)
		}
	}

	// Gather metrics
	mfs, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	// Verify no metrics were recorded for non-evicted pods
	for _, mf := range mfs {
		if mf.GetName() == "evicted_pods_deleted_total" || mf.GetName() == "evicted_pods_skipped_total" {
			if len(mf.GetMetric()) > 0 {
				t.Errorf("Expected no metrics for %s, but found %d", mf.GetName(), len(mf.GetMetric()))
			}
		}
	}
}
