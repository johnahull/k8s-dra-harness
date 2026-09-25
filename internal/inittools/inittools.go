// Package inittools creates the globals every suite dot-imports: APIClient and
// Config. Config is loaded, completed with platform defaults, and validated
// before any spec runs, so bad settings fail immediately.
package inittools

import (
	"flag"

	"github.com/golang/glog"
	"github.com/johnahull/amd-gpu-e2e/internal/config"
	"github.com/johnahull/amd-gpu-e2e/internal/platform"
	"github.com/johnahull/amd-gpu-e2e/pkg/clients"
	ginkgo "github.com/onsi/ginkgo/v2"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	// APIClient provides access to the cluster.
	APIClient *clients.Settings
	// Config holds validated amd-gpu-e2e settings.
	Config *config.Config
)

func init() {
	logf.SetLogger(zap.New(zap.WriteTo(ginkgo.GinkgoWriter), zap.UseDevMode(true)))

	var err error

	if Config, err = config.Load(); err != nil {
		glog.Fatalf("loading config: %v", err)
	}

	_ = flag.Lookup("logtostderr").Value.Set("true")
	_ = flag.Lookup("v").Value.Set(Config.VerboseLevel)

	if APIClient, err = clients.New(""); err != nil {
		glog.Fatalf("creating API client (check KUBECONFIG): %v", err)
	}

	p, err := platform.Detect(APIClient.Discovery, Config.Platform)
	if err != nil {
		glog.Fatalf("detecting platform: %v", err)
	}

	Config.ApplyPlatformDefaults(p)

	if err := Config.Validate(); err != nil {
		glog.Fatalf("invalid configuration:\n%v", err)
	}
}
