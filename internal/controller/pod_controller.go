package controller

import (
	"context"
	"time"

	"github.com/kyosenergy/evicted-pod-reaper/internal/metrics"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	preserveAnnotation = "pod-reaper.kyos.com/preserve"
)

// PodReconciler reconciles a Pod object
type PodReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	Metrics     *metrics.PodMetrics
	TTLToDelete int // seconds to wait before deletion
}

//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;delete
//+kubebuilder:rbac:groups="",resources=pods/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop
func (r *PodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the Pod instance
	pod := &corev1.Pod{}
	err := r.Get(ctx, req.NamespacedName, pod)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return without error
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch Pod")
		return ctrl.Result{}, err
	}

	// Check if pod is evicted
	if !r.isPodEvicted(pod) {
		log.V(1).Info("pod is not evicted, skipping", "phase", pod.Status.Phase, "reason", pod.Status.Reason)
		return ctrl.Result{}, nil
	}

	// Check preservation annotation
	if r.shouldPreservePod(pod) {
		log.Info("pod has preserve annotation, skipping deletion", "pod", req.NamespacedName)
		r.Metrics.IncSkipped(pod.Namespace)
		return ctrl.Result{}, nil
	}

	// Check TTL
	if !r.hasExceededTTL(pod) {
		requeueAfter := r.calculateRequeueTime(pod)
		log.Info("pod has not exceeded TTL, requeuing", "pod", req.NamespacedName, "requeueAfter", requeueAfter)
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	// Delete the pod
	log.Info("deleting evicted pod", "pod", req.NamespacedName)
	if err := r.Delete(ctx, pod); err != nil {
		log.Error(err, "unable to delete pod", "pod", req.NamespacedName)
		return ctrl.Result{}, err
	}

	r.Metrics.IncDeleted(pod.Namespace)
	log.Info("successfully deleted evicted pod", "pod", req.NamespacedName)

	return ctrl.Result{}, nil
}

// isPodEvicted checks if a pod is in evicted state
func (r *PodReconciler) isPodEvicted(pod *corev1.Pod) bool {
	return pod.Status.Phase == corev1.PodFailed && pod.Status.Reason == "Evicted"
}

// shouldPreservePod checks if pod has preserve annotation set to "true"
func (r *PodReconciler) shouldPreservePod(pod *corev1.Pod) bool {
	if pod.Annotations == nil {
		return false
	}
	return pod.Annotations[preserveAnnotation] == "true"
}

// hasExceededTTL checks if the pod has exceeded the TTL
func (r *PodReconciler) hasExceededTTL(pod *corev1.Pod) bool {
	if pod.Status.StartTime == nil {
		// If no start time, consider it exceeded
		return true
	}

	podAge := time.Since(pod.Status.StartTime.Time)
	return podAge > time.Duration(r.TTLToDelete)*time.Second
}

// calculateRequeueTime calculates when to requeue the pod for deletion
func (r *PodReconciler) calculateRequeueTime(pod *corev1.Pod) time.Duration {
	if pod.Status.StartTime == nil {
		return 0
	}

	podAge := time.Since(pod.Status.StartTime.Time)
	ttlDuration := time.Duration(r.TTLToDelete) * time.Second

	if podAge >= ttlDuration {
		return 0
	}

	return ttlDuration - podAge
}

// SetupWithManager sets up the controller with the Manager.
func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Only watch pods that are Failed
	failedPredicate := predicate.NewPredicateFuncs(func(obj client.Object) bool {
		pod, ok := obj.(*corev1.Pod)
		if !ok {
			return false
		}
		return pod.Status.Phase == corev1.PodFailed
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		WithEventFilter(failedPredicate).
		Complete(r)
}
