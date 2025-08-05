package controller

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kyosenergy/evicted-pod-reaper/internal/metrics"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Test edge cases for better coverage
func TestPodReconciler_EdgeCases(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)

	t.Run("pod not found error", func(t *testing.T) {
		// Create empty fake client
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		r := &PodReconciler{
			Client:      fakeClient,
			Scheme:      scheme,
			Metrics:     metrics.NewPodMetrics(),
			TTLToDelete: 300,
		}

		// Try to reconcile non-existent pod
		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "non-existent",
				Namespace: "default",
			},
		}
		result, err := r.Reconcile(context.Background(), req)

		// Should return success with no error (pod not found is expected)
		if err != nil {
			t.Errorf("Expected no error for non-existent pod, got: %v", err)
		}
		if result != (ctrl.Result{}) {
			t.Errorf("Expected empty result, got: %v", result)
		}
	})

	t.Run("evicted pod without start time", func(t *testing.T) {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod-no-start-time",
				Namespace: "default",
			},
			Status: corev1.PodStatus{
				Phase:     corev1.PodFailed,
				Reason:    "Evicted",
				StartTime: nil, // No start time
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithRuntimeObjects(pod).
			Build()

		r := &PodReconciler{
			Client:      fakeClient,
			Scheme:      scheme,
			Metrics:     metrics.NewPodMetrics(),
			TTLToDelete: 300,
		}

		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      pod.Name,
				Namespace: pod.Namespace,
			},
		}
		result, err := r.Reconcile(context.Background(), req)

		if err != nil {
			t.Errorf("Reconcile() error = %v", err)
		}

		// Should delete immediately when no start time
		if result != (ctrl.Result{}) {
			t.Errorf("Expected empty result (pod deleted), got: %v", result)
		}

		// Verify pod was deleted
		err = fakeClient.Get(context.Background(), req.NamespacedName, &corev1.Pod{})
		if err == nil {
			t.Errorf("Expected pod to be deleted, but it still exists")
		}
	})
}

func TestPodReconciler_hasExceededTTL_NoStartTime(t *testing.T) {
	r := &PodReconciler{TTLToDelete: 300}

	pod := &corev1.Pod{
		Status: corev1.PodStatus{
			StartTime: nil,
		},
	}

	// Should return true when no start time
	if !r.hasExceededTTL(pod) {
		t.Error("hasExceededTTL() should return true when pod has no start time")
	}
}

func TestPodReconciler_calculateRequeueTime_NoStartTime(t *testing.T) {
	r := &PodReconciler{TTLToDelete: 300}

	pod := &corev1.Pod{
		Status: corev1.PodStatus{
			StartTime: nil,
		},
	}

	// Should return 0 when no start time
	if r.calculateRequeueTime(pod) != 0 {
		t.Error("calculateRequeueTime() should return 0 when pod has no start time")
	}
}

func TestPodReconciler_calculateRequeueTime_AlreadyExceeded(t *testing.T) {
	r := &PodReconciler{TTLToDelete: 300}

	pod := &corev1.Pod{
		Status: corev1.PodStatus{
			StartTime: &metav1.Time{Time: time.Now().Add(-10 * time.Minute)}, // Already exceeded
		},
	}

	// Should return 0 when already exceeded
	if r.calculateRequeueTime(pod) != 0 {
		t.Error("calculateRequeueTime() should return 0 when TTL already exceeded")
	}
}

// Test client errors during reconciliation
type errorClient struct {
	client.Client
	getError    error
	deleteError error
}

func (c *errorClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if c.getError != nil {
		return c.getError
	}
	// Return a pod for testing
	if pod, ok := obj.(*corev1.Pod); ok {
		pod.Name = key.Name
		pod.Namespace = key.Namespace
		pod.Status.Phase = corev1.PodFailed
		pod.Status.Reason = "Evicted"
		pod.Status.StartTime = &metav1.Time{Time: time.Now().Add(-10 * time.Minute)}
	}
	return nil
}

func (c *errorClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	return c.deleteError
}

func TestPodReconciler_ClientErrors(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)

	t.Run("get error", func(t *testing.T) {
		r := &PodReconciler{
			Client:      &errorClient{getError: errors.New("get failed")},
			Scheme:      scheme,
			Metrics:     metrics.NewPodMetrics(),
			TTLToDelete: 300,
		}

		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "test-pod",
				Namespace: "default",
			},
		}
		_, err := r.Reconcile(context.Background(), req)

		if err == nil || err.Error() != "get failed" {
			t.Errorf("Expected 'get failed' error, got: %v", err)
		}
	})

	t.Run("delete error", func(t *testing.T) {
		r := &PodReconciler{
			Client:      &errorClient{deleteError: errors.New("delete failed")},
			Scheme:      scheme,
			Metrics:     metrics.NewPodMetrics(),
			TTLToDelete: 300,
		}

		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "test-pod",
				Namespace: "default",
			},
		}
		_, err := r.Reconcile(context.Background(), req)

		if err == nil || err.Error() != "delete failed" {
			t.Errorf("Expected 'delete failed' error, got: %v", err)
		}
	})
}
