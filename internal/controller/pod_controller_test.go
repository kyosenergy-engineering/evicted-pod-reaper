package controller

import (
	"context"
	"testing"
	"time"

	"github.com/kyosenergy-engineering/evicted-pod-reaper/internal/metrics"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestPodReconciler_Reconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)

	tests := []struct {
		name       string
		pod        *corev1.Pod
		ttl        int
		wantResult ctrl.Result
		wantError  bool
		wantDelete bool
	}{
		{
			name: "evicted pod should be deleted after TTL",
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
			ttl:        300, // 5 minutes
			wantResult: ctrl.Result{},
			wantError:  false,
			wantDelete: true,
		},
		{
			name: "evicted pod with preserve annotation should be skipped",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod-preserve",
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
			ttl:        300,
			wantResult: ctrl.Result{},
			wantError:  false,
			wantDelete: false,
		},
		{
			name: "evicted pod before TTL should be requeued",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod-new",
					Namespace: "default",
				},
				Status: corev1.PodStatus{
					Phase:     corev1.PodFailed,
					Reason:    "Evicted",
					StartTime: &metav1.Time{Time: time.Now().Add(-1 * time.Minute)},
				},
			},
			ttl:        300,                                        // 5 minutes
			wantResult: ctrl.Result{RequeueAfter: 4 * time.Minute}, // approximately
			wantError:  false,
			wantDelete: false,
		},
		{
			name: "running pod should be ignored",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod-running",
					Namespace: "default",
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			},
			ttl:        300,
			wantResult: ctrl.Result{},
			wantError:  false,
			wantDelete: false,
		},
		{
			name: "failed pod with different reason should be ignored",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod-failed",
					Namespace: "default",
				},
				Status: corev1.PodStatus{
					Phase:  corev1.PodFailed,
					Reason: "OOMKilled",
				},
			},
			ttl:        300,
			wantResult: ctrl.Result{},
			wantError:  false,
			wantDelete: false,
		},
		{
			name: "evicted pod with preserve annotation false should be deleted",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod-preserve-false",
					Namespace: "default",
					Annotations: map[string]string{
						"pod-reaper.kyos.com/preserve": "false",
					},
				},
				Status: corev1.PodStatus{
					Phase:     corev1.PodFailed,
					Reason:    "Evicted",
					StartTime: &metav1.Time{Time: time.Now().Add(-10 * time.Minute)},
				},
			},
			ttl:        300,
			wantResult: ctrl.Result{},
			wantError:  false,
			wantDelete: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake client with the pod
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(tt.pod).
				Build()

			// Create reconciler
			r := &PodReconciler{
				Client:      fakeClient,
				Scheme:      scheme,
				Metrics:     metrics.NewPodMetrics(),
				TTLToDelete: tt.ttl,
			}

			// Run reconcile
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      tt.pod.Name,
					Namespace: tt.pod.Namespace,
				},
			}
			result, err := r.Reconcile(context.Background(), req)

			// Check error
			if (err != nil) != tt.wantError {
				t.Errorf("Reconcile() error = %v, wantError %v", err, tt.wantError)
			}

			// Check result (allow some time difference for requeue)
			if tt.wantResult.RequeueAfter > 0 {
				if result.RequeueAfter < tt.wantResult.RequeueAfter-time.Minute ||
					result.RequeueAfter > tt.wantResult.RequeueAfter+time.Minute {
					t.Errorf("Reconcile() result.RequeueAfter = %v, want approximately %v", result.RequeueAfter, tt.wantResult.RequeueAfter)
				}
			} else if result != tt.wantResult {
				t.Errorf("Reconcile() result = %v, want %v", result, tt.wantResult)
			}

			// Check if pod was deleted
			pod := &corev1.Pod{}
			err = fakeClient.Get(context.Background(), req.NamespacedName, pod)
			if tt.wantDelete && err == nil {
				t.Errorf("Expected pod to be deleted, but it still exists")
			}
			if !tt.wantDelete && err != nil {
				t.Errorf("Expected pod to exist, but got error: %v", err)
			}
		})
	}
}

func TestPodReconciler_isPodEvicted(t *testing.T) {
	r := &PodReconciler{}

	tests := []struct {
		name string
		pod  *corev1.Pod
		want bool
	}{
		{
			name: "evicted pod",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase:  corev1.PodFailed,
					Reason: "Evicted",
				},
			},
			want: true,
		},
		{
			name: "running pod",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			},
			want: false,
		},
		{
			name: "failed pod with different reason",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase:  corev1.PodFailed,
					Reason: "OOMKilled",
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := r.isPodEvicted(tt.pod); got != tt.want {
				t.Errorf("isPodEvicted() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPodReconciler_shouldPreservePod(t *testing.T) {
	r := &PodReconciler{}

	tests := []struct {
		name string
		pod  *corev1.Pod
		want bool
	}{
		{
			name: "pod with preserve annotation true",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"pod-reaper.kyos.com/preserve": "true",
					},
				},
			},
			want: true,
		},
		{
			name: "pod with preserve annotation false",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"pod-reaper.kyos.com/preserve": "false",
					},
				},
			},
			want: false,
		},
		{
			name: "pod without annotations",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{},
			},
			want: false,
		},
		{
			name: "pod with empty annotations",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := r.shouldPreservePod(tt.pod); got != tt.want {
				t.Errorf("shouldPreservePod() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestPodReconciler_EvictedPredicate tests the predicate used in SetupWithManager
func TestPodReconciler_EvictedPredicate(t *testing.T) {
	tests := []struct {
		name string
		pod  *corev1.Pod
		want bool
	}{
		{
			name: "evicted pod should match predicate",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "evicted-pod",
					Namespace: "default",
				},
				Status: corev1.PodStatus{
					Phase:  corev1.PodFailed,
					Reason: "Evicted",
				},
			},
			want: true,
		},
		{
			name: "failed pod with different reason should not match predicate",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "oom-killed-pod",
					Namespace: "default",
				},
				Status: corev1.PodStatus{
					Phase:  corev1.PodFailed,
					Reason: "OOMKilled",
				},
			},
			want: false,
		},
		{
			name: "running pod should not match predicate",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "running-pod",
					Namespace: "default",
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			},
			want: false,
		},
		{
			name: "pending pod should not match predicate",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pending-pod",
					Namespace: "default",
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
				},
			},
			want: false,
		},
		{
			name: "succeeded pod should not match predicate",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "succeeded-pod",
					Namespace: "default",
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodSucceeded,
				},
			},
			want: false,
		},
		{
			name: "failed pod with empty reason should not match predicate",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "failed-pod-no-reason",
					Namespace: "default",
				},
				Status: corev1.PodStatus{
					Phase:  corev1.PodFailed,
					Reason: "",
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use the shared predicate function from the controller
			got := isEvictedPodPredicate(tt.pod)
			if got != tt.want {
				t.Errorf("isEvictedPodPredicate() = %v, want %v", got, tt.want)
			}
		})
	}
}
