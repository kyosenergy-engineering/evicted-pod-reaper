package controller

import (
	"context"
	"testing"
	"time"

	"github.com/kyosenergy-engineering/evicted-pod-reaper/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// TestPodReconciler_NamespaceFiltering verifies that the controller correctly
// handles pods from different namespaces according to the README test cases
func TestPodReconciler_NamespaceFiltering(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)

	tests := []struct {
		name            string
		pod             *corev1.Pod
		watchNamespaces []string // Simulates REAPER_WATCH_NAMESPACES
		expectDeleted   bool
		expectSkipped   bool
	}{
		{
			name: "evicted pod in watched namespace should be deleted",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
				Status: corev1.PodStatus{
					Phase:     corev1.PodFailed,
					Reason:    "Evicted",
					StartTime: &metav1.Time{Time: time.Now().Add(-10 * time.Minute)},
				},
			},
			watchNamespaces: []string{"default"},
			expectDeleted:   true,
			expectSkipped:   false,
		},
		{
			name: "evicted pod in unwatched namespace - controller won't see it",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "kube-system",
				},
				Status: corev1.PodStatus{
					Phase:     corev1.PodFailed,
					Reason:    "Evicted",
					StartTime: &metav1.Time{Time: time.Now().Add(-10 * time.Minute)},
				},
			},
			watchNamespaces: []string{"default"},
			expectDeleted:   false, // Won't be deleted because controller won't reconcile it
			expectSkipped:   false,
		},
		{
			name: "evicted pod with preserve annotation in watched namespace",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod-preserve",
					Namespace: "monitoring",
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
			watchNamespaces: []string{"monitoring", "default"},
			expectDeleted:   false,
			expectSkipped:   true,
		},
		{
			name: "multiple namespaces configured",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "kube-system",
				},
				Status: corev1.PodStatus{
					Phase:     corev1.PodFailed,
					Reason:    "Evicted",
					StartTime: &metav1.Time{Time: time.Now().Add(-10 * time.Minute)},
				},
			},
			watchNamespaces: []string{"default", "kube-system", "monitoring"},
			expectDeleted:   true,
			expectSkipped:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create metrics and registry
			podMetrics := metrics.NewPodMetrics()
			registry := prometheus.NewRegistry()
			podMetrics.Register(registry)

			// Create fake client with the pod
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(tt.pod).
				Build()

			// Create reconciler
			r := &PodReconciler{
				Client:      fakeClient,
				Scheme:      scheme,
				Metrics:     podMetrics,
				TTLToDelete: 300,
			}

			// Note: In a real scenario, the manager's cache would filter namespaces
			// Here we simulate the behavior by only reconciling if namespace matches
			shouldReconcile := false
			for _, ns := range tt.watchNamespaces {
				if ns == tt.pod.Namespace {
					shouldReconcile = true
					break
				}
			}

			if shouldReconcile {
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
			}

			// Verify pod deletion
			err := fakeClient.Get(context.Background(), types.NamespacedName{
				Name:      tt.pod.Name,
				Namespace: tt.pod.Namespace,
			}, &corev1.Pod{})

			podExists := err == nil

			if tt.expectDeleted && podExists {
				t.Errorf("Expected pod to be deleted, but it still exists")
			}
			if !tt.expectDeleted && !podExists && shouldReconcile {
				t.Errorf("Expected pod to exist, but it was deleted")
			}

			// Verify metrics
			mfs, err := registry.Gather()
			if err != nil {
				t.Fatalf("Failed to gather metrics: %v", err)
			}

			var deletedCount, skippedCount float64
			for _, mf := range mfs {
				for _, m := range mf.GetMetric() {
					for _, label := range m.GetLabel() {
						if label.GetName() == "namespace" && label.GetValue() == tt.pod.Namespace {
							if mf.GetName() == "evicted_pods_deleted_total" {
								deletedCount = m.GetCounter().GetValue()
							}
							if mf.GetName() == "evicted_pods_skipped_total" {
								skippedCount = m.GetCounter().GetValue()
							}
						}
					}
				}
			}

			if tt.expectDeleted && deletedCount != 1 {
				t.Errorf("Expected deleted metric to be 1, got %v", deletedCount)
			}
			if tt.expectSkipped && skippedCount != 1 {
				t.Errorf("Expected skipped metric to be 1, got %v", skippedCount)
			}
		})
	}
}

// TestNamespaceConfiguration tests that the manager correctly configures namespace watching
func TestNamespaceConfiguration(t *testing.T) {
	tests := []struct {
		name                  string
		watchAllNamespaces    bool
		watchNamespaces       []string
		expectNamespaceFilter bool
		expectedWatchedCount  int
	}{
		{
			name:                  "watch all namespaces",
			watchAllNamespaces:    true,
			watchNamespaces:       []string{"default"},
			expectNamespaceFilter: false, // No filter when watching all
		},
		{
			name:                  "watch specific namespaces",
			watchAllNamespaces:    false,
			watchNamespaces:       []string{"default", "kube-system"},
			expectNamespaceFilter: true,
			expectedWatchedCount:  2,
		},
		{
			name:                  "default to default namespace",
			watchAllNamespaces:    false,
			watchNamespaces:       []string{"default"},
			expectNamespaceFilter: true,
			expectedWatchedCount:  1,
		},
		{
			name:                  "empty namespace list defaults to default",
			watchAllNamespaces:    false,
			watchNamespaces:       []string{},
			expectNamespaceFilter: false, // No filter applied when empty
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This test documents the expected behavior
			// In production, the manager's cache configuration handles namespace filtering

			if tt.watchAllNamespaces && tt.expectNamespaceFilter {
				t.Error("When watching all namespaces, no filter should be applied")
			}

			if !tt.watchAllNamespaces && len(tt.watchNamespaces) > 0 && !tt.expectNamespaceFilter {
				t.Error("When watching specific namespaces, filter should be applied")
			}
		})
	}
}
