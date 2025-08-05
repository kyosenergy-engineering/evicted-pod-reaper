package main

import (
	"flag"
	"os"
	"strconv"
	"strings"

	"github.com/kyosenergy/evicted-pod-reaper/internal/controller"
	"github.com/kyosenergy/evicted-pod-reaper/internal/metrics"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// Parse environment variables
	watchAllNamespaces := os.Getenv("REAPER_WATCH_ALL_NAMESPACES") == "true"
	watchNamespaces := parseNamespaces(os.Getenv("REAPER_WATCH_NAMESPACES"))
	ttlToDelete := parseTTL(os.Getenv("REAPER_TTL_TO_DELETE"))

	setupLog.Info("Starting evicted-pod-reaper",
		"watchAllNamespaces", watchAllNamespaces,
		"watchNamespaces", watchNamespaces,
		"ttlToDelete", ttlToDelete,
	)

	// Configure manager options
	mgrOpts := ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "evicted-pod-reaper.kyos.com",
	}

	// Configure namespace watching
	if !watchAllNamespaces && len(watchNamespaces) > 0 {
		mgrOpts.Cache = cache.Options{
			DefaultNamespaces: make(map[string]cache.Config),
		}
		for _, ns := range watchNamespaces {
			mgrOpts.Cache.DefaultNamespaces[ns] = cache.Config{}
		}
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), mgrOpts)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Register metrics
	podMetrics := metrics.NewPodMetrics()
	podMetrics.Register(ctrlmetrics.Registry)

	// Setup controller
	if err = (&controller.PodReconciler{
		Client:      mgr.GetClient(),
		Scheme:      mgr.GetScheme(),
		Metrics:     podMetrics,
		TTLToDelete: ttlToDelete,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Pod")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func parseNamespaces(env string) []string {
	if env == "" {
		return []string{"default"}
	}
	namespaces := strings.Split(env, ",")
	for i := range namespaces {
		namespaces[i] = strings.TrimSpace(namespaces[i])
	}
	return namespaces
}

func parseTTL(env string) int {
	if env == "" {
		return 300 // default 5 minutes
	}
	ttl, err := strconv.Atoi(env)
	if err != nil {
		setupLog.Error(err, "invalid TTL value, using default", "value", env)
		return 300
	}
	return ttl
}
