//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/common/expfmt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

const (
	testNamespace = "test-evicted-pods"
	ttlSeconds    = 5
)

var (
	kubeconfig string
)

func TestE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping e2e test in short mode")
	}

	// Get kubeconfig
	kubeconfig = os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		home, _ := os.UserHomeDir()
		kubeconfig = home + "/.kube/config"
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		t.Fatalf("Failed to build config: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatalf("Failed to create clientset: %v", err)
	}

	ctx := context.Background()

	// Ensure test namespace exists
	_, err = clientset.CoreV1().Namespaces().Get(ctx, testNamespace, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Test namespace %s does not exist. Make sure the controller is deployed.", testNamespace)
	}

	// Run test scenarios
	t.Run("EvictedPodShouldBeDeleted", func(t *testing.T) {
		testEvictedPodDeletion(t, ctx, clientset)
	})

	t.Run("PreservedPodShouldNotBeDeleted", func(t *testing.T) {
		testPreservedPod(t, ctx, clientset)
	})

	t.Run("RunningPodShouldNotBeDeleted", func(t *testing.T) {
		testRunningPod(t, ctx, clientset)
	})

	t.Run("PodInUnwatchedNamespaceShouldNotBeDeleted", func(t *testing.T) {
		testUnwatchedNamespace(t, ctx, clientset)
	})

	t.Run("MetricsEndpointShouldWork", func(t *testing.T) {
		testMetricsEndpoint(t, ctx, clientset)
	})
}

func testEvictedPodDeletion(t *testing.T, ctx context.Context, clientset *kubernetes.Clientset) {
	podName := "test-evicted-pod-1"

	// Create a pod that will be evicted
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: testNamespace,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    "test",
					Image:   "busybox",
					Command: []string{"sh", "-c", "exit 1"},
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}

	_, err := clientset.CoreV1().Pods(testNamespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create pod: %v", err)
	}

	// Wait for pod to fail
	err = wait.PollImmediate(time.Second, 30*time.Second, func() (bool, error) {
		pod, err := clientset.CoreV1().Pods(testNamespace).Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		return pod.Status.Phase == corev1.PodFailed, nil
	})
	if err != nil {
		t.Fatalf("Pod did not fail in time: %v", err)
	}

	// Simulate eviction by patching the pod status
	pod, _ = clientset.CoreV1().Pods(testNamespace).Get(ctx, podName, metav1.GetOptions{})
	pod.Status.Phase = corev1.PodFailed
	pod.Status.Reason = "Evicted"
	pod.Status.Message = "The node was low on resource: memory."
	_, err = clientset.CoreV1().Pods(testNamespace).UpdateStatus(ctx, pod, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("Failed to update pod status: %v", err)
	}

	// Wait for pod to be deleted (should happen within TTL + some buffer)
	err = wait.PollImmediate(time.Second, time.Duration(ttlSeconds+10)*time.Second, func() (bool, error) {
		_, err := clientset.CoreV1().Pods(testNamespace).Get(ctx, podName, metav1.GetOptions{})
		if err != nil && strings.Contains(err.Error(), "not found") {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		t.Errorf("Evicted pod was not deleted within expected time: %v", err)
	}
}

func testPreservedPod(t *testing.T, ctx context.Context, clientset *kubernetes.Clientset) {
	podName := "test-preserved-pod"

	// Create a pod with preserve annotation
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: testNamespace,
			Annotations: map[string]string{
				"pod-reaper.kyos.com/preserve": "true",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    "test",
					Image:   "busybox",
					Command: []string{"sh", "-c", "exit 1"},
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}

	_, err := clientset.CoreV1().Pods(testNamespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create pod: %v", err)
	}

	// Wait for pod to fail
	err = wait.PollImmediate(time.Second, 30*time.Second, func() (bool, error) {
		pod, err := clientset.CoreV1().Pods(testNamespace).Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		return pod.Status.Phase == corev1.PodFailed, nil
	})
	if err != nil {
		t.Fatalf("Pod did not fail in time: %v", err)
	}

	// Simulate eviction
	pod, _ = clientset.CoreV1().Pods(testNamespace).Get(ctx, podName, metav1.GetOptions{})
	pod.Status.Phase = corev1.PodFailed
	pod.Status.Reason = "Evicted"
	pod.Status.Message = "The node was low on resource: memory."
	_, err = clientset.CoreV1().Pods(testNamespace).UpdateStatus(ctx, pod, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("Failed to update pod status: %v", err)
	}

	// Wait longer than TTL and verify pod still exists
	time.Sleep(time.Duration(ttlSeconds+3) * time.Second)

	_, err = clientset.CoreV1().Pods(testNamespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		t.Errorf("Preserved pod was deleted but should have been kept: %v", err)
	}

	// Cleanup
	_ = clientset.CoreV1().Pods(testNamespace).Delete(ctx, podName, metav1.DeleteOptions{})
}

func testRunningPod(t *testing.T, ctx context.Context, clientset *kubernetes.Clientset) {
	podName := "test-running-pod"

	// Create a running pod
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: testNamespace,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    "test",
					Image:   "busybox",
					Command: []string{"sleep", "3600"},
				},
			},
		},
	}

	_, err := clientset.CoreV1().Pods(testNamespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create pod: %v", err)
	}

	// Wait for pod to be running
	err = wait.PollImmediate(time.Second, 30*time.Second, func() (bool, error) {
		pod, err := clientset.CoreV1().Pods(testNamespace).Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		return pod.Status.Phase == corev1.PodRunning, nil
	})
	if err != nil {
		t.Fatalf("Pod did not start running in time: %v", err)
	}

	// Wait longer than TTL and verify pod still exists
	time.Sleep(time.Duration(ttlSeconds+3) * time.Second)

	_, err = clientset.CoreV1().Pods(testNamespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		t.Errorf("Running pod was deleted but should have been kept: %v", err)
	}

	// Cleanup
	_ = clientset.CoreV1().Pods(testNamespace).Delete(ctx, podName, metav1.DeleteOptions{})
}

func testUnwatchedNamespace(t *testing.T, ctx context.Context, clientset *kubernetes.Clientset) {
	unwatchedNS := "unwatched-namespace"
	podName := "test-unwatched-pod"

	// Create namespace if it doesn't exist
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: unwatchedNS,
		},
	}
	_, err := clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("Failed to create namespace: %v", err)
	}

	// Create a pod in unwatched namespace
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: unwatchedNS,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    "test",
					Image:   "busybox",
					Command: []string{"sh", "-c", "exit 1"},
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}

	_, err = clientset.CoreV1().Pods(unwatchedNS).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create pod: %v", err)
	}

	// Wait for pod to fail
	err = wait.PollImmediate(time.Second, 30*time.Second, func() (bool, error) {
		pod, err := clientset.CoreV1().Pods(unwatchedNS).Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		return pod.Status.Phase == corev1.PodFailed, nil
	})
	if err != nil {
		t.Fatalf("Pod did not fail in time: %v", err)
	}

	// Simulate eviction
	pod, _ = clientset.CoreV1().Pods(unwatchedNS).Get(ctx, podName, metav1.GetOptions{})
	pod.Status.Phase = corev1.PodFailed
	pod.Status.Reason = "Evicted"
	_, err = clientset.CoreV1().Pods(unwatchedNS).UpdateStatus(ctx, pod, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("Failed to update pod status: %v", err)
	}

	// Wait longer than TTL and verify pod still exists
	time.Sleep(time.Duration(ttlSeconds+3) * time.Second)

	_, err = clientset.CoreV1().Pods(unwatchedNS).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		t.Errorf("Pod in unwatched namespace was deleted but should have been kept: %v", err)
	}

	// Cleanup
	_ = clientset.CoreV1().Pods(unwatchedNS).Delete(ctx, podName, metav1.DeleteOptions{})
	_ = clientset.CoreV1().Namespaces().Delete(ctx, unwatchedNS, metav1.DeleteOptions{})
}

func testMetricsEndpoint(t *testing.T, ctx context.Context, clientset *kubernetes.Clientset) {
	// Get the pod running the controller
	pods, err := clientset.CoreV1().Pods(testNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app=evicted-pod-reaper",
	})
	if err != nil || len(pods.Items) == 0 {
		t.Fatalf("Failed to find controller pod: %v", err)
	}

	podName := pods.Items[0].Name

	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		t.Fatalf("Failed to build rest config: %v", err)
	}

	// Find the pod's port (assume 8080)
	localPort := "18080"
	podPort := "8080"

	// Create port forwarder
	url := fmt.Sprintf("%s/api/v1/namespaces/%s/pods/%s/portforward", restConfig.Host, testNamespace, podName)
	transport, upgrader, err := spdy.RoundTripperFor(restConfig)
	if err != nil {
		t.Fatalf("Failed to create roundtripper: %v", err)
	}

	stopChan := make(chan struct{}, 1)
	readyChan := make(chan struct{}, 1)
	defer close(stopChan)

	pf, err := portforward.New(
		spdy.NewDialer(upgrader, &http.Client{Transport: transport}, "POST", mustParseURL(url)),
		[]string{localPort + ":" + podPort},
		stopChan,
		readyChan,
		io.Discard,
		io.Discard,
	)
	if err != nil {
		t.Fatalf("Failed to create port forward: %v", err)
	}

	go func() {
		_ = pf.ForwardPorts()
	}()

	// Wait for port-forward to be ready
	select {
	case <-readyChan:
		// ready
	case <-time.After(10 * time.Second):
		t.Fatalf("Port-forward did not become ready in time")
	}

	// Use local port to access metrics
	resp, err := http.Get(fmt.Sprintf("http://localhost:%s/metrics", localPort))
	if err != nil {
		t.Fatalf("Could not access metrics endpoint via port-forward: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read metrics response: %v", err)
	}

	parser := expfmt.TextParser{}
	metricFamilies, err := parser.TextToMetricFamilies(strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("Failed to parse Prometheus metrics: %v", err)
	}

	if _, ok := metricFamilies["evicted_pods_deleted_total"]; !ok {
		t.Errorf("Metrics endpoint does not contain expected metric 'evicted_pods_deleted_total'")
	}
	if _, ok := metricFamilies["evicted_pods_skipped_total"]; !ok {
		t.Errorf("Metrics endpoint does not contain expected metric 'evicted_pods_skipped_total'")
	}
}

// mustParseURL is a helper to panic on invalid URL (for test only)
func mustParseURL(rawurl string) *url.URL {
	u, err := url.Parse(rawurl)
	if err != nil {
		panic(err)
	}
	return u
}
