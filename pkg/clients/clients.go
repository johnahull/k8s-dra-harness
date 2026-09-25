// Package clients builds the Kubernetes clients shared by every suite.
package clients

import (
	"fmt"
	"os"

	amdv1alpha1 "github.com/johnahull/amd-ci/pkg/amdgpu/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Settings bundles the clients for one cluster. The embedded controller-runtime
// client is the default way to read and write objects. OpenShift-only schemes
// are added by internal/platform (Plan 2), never required here.
type Settings struct {
	client.Client

	Config    *rest.Config
	K8s       kubernetes.Interface
	Discovery discovery.DiscoveryInterface
}

// NewScheme returns a scheme with the core Kubernetes types (including
// resource.k8s.io/v1) and amd.com/v1alpha1.
func NewScheme() (*runtime.Scheme, error) {
	s := runtime.NewScheme()

	if err := clientgoscheme.AddToScheme(s); err != nil {
		return nil, fmt.Errorf("adding client-go scheme: %w", err)
	}

	if err := amdv1alpha1.AddToScheme(s); err != nil {
		return nil, fmt.Errorf("adding amd.com/v1alpha1 scheme: %w", err)
	}

	return s, nil
}

// New connects using kubeconfig, falling back to $KUBECONFIG and then to
// in-cluster config.
func New(kubeconfig string) (*Settings, error) {
	if kubeconfig == "" {
		kubeconfig = os.Getenv("KUBECONFIG")
	}

	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("loading kubeconfig %q: %w", kubeconfig, err)
	}

	k8s, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating clientset: %w", err)
	}

	scheme, err := NewScheme()
	if err != nil {
		return nil, err
	}

	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("creating controller-runtime client: %w", err)
	}

	return &Settings{Client: c, Config: cfg, K8s: k8s, Discovery: k8s.Discovery()}, nil
}
