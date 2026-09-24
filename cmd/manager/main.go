package main

import (
	"flag"
	"os"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	jevletv1alpha1 "github.com/slateeho/jevlet/api/v1alpha1"
	"github.com/slateeho/jevlet/internal/controller"
	"github.com/slateeho/jevlet/internal/jev"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(jevletv1alpha1.AddToScheme(scheme))
}

func main() {
	var metricsAddr, probeAddr string
	var leaderElect bool
	flag.StringVar(&metricsAddr, "metrics-bind-address", envOr("METRICS_BIND_ADDRESS", ":8080"), "metrics endpoint")
	flag.StringVar(&probeAddr, "health-probe-bind-address", envOr("HEALTH_PROBE_BIND_ADDRESS", ":8081"), "health probe endpoint")
	flag.BoolVar(&leaderElect, "leader-elect", strings.EqualFold(os.Getenv("LEADER_ELECT"), "true"), "enable leader election")
	zapOpts := zap.Options{Development: true}
	zapOpts.BindFlags(flag.CommandLine)
	flag.Parse()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&zapOpts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         leaderElect,
		LeaderElectionID:       "jevlet-controller.jevlet.io",
	})
	if err != nil {
		ctrl.Log.Error(err, "unable to start manager")
		os.Exit(1)
	}

	jc := &jev.Client{
		BaseURL: envOr("JEV_BASE_URL", "https://api.typesafe.ai"),
		APIKey:  os.Getenv("TYPESAFE_API_KEY"),
		Model:   envOr("JEV_MODEL", "jev-latest"),
	}
	if err := (&controller.PodReconciler{Client: mgr.GetClient(), APIReader: mgr.GetAPIReader(), Jev: jc, Now: time.Now}).SetupWithManager(mgr); err != nil {
		ctrl.Log.Error(err, "unable to create Pod controller")
		os.Exit(1)
	}
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		ctrl.Log.Error(err, "healthz")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		ctrl.Log.Error(err, "readyz")
		os.Exit(1)
	}
	ctrl.Log.Info("starting Jevlet")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		ctrl.Log.Error(err, "manager stopped")
		os.Exit(1)
	}
}

func envOr(name, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return fallback
}
