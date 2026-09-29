// Package clients builds the Kubernetes clients shared by every suite.
package clients

import (
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	kubevirtclient "kubevirt.io/client-go/kubevirt"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Settings bundles the clients for one cluster. The embedded controller-runtime
// client is the default way to read and write objects. OpenShift-only schemes
// are added by internal/platform (Plan 2), never required here.
type Settings struct {
	client.Client

	Config     *rest.Config
	K8s        kubernetes.Interface
	Dynamic    dynamic.Interface
	Kubevirt   kubevirtclient.Interface
	Discovery  discovery.DiscoveryInterface
	Kubeconfig string
}

// NewScheme returns a core Kubernetes scheme. Adapters register their own APIs.
func NewScheme(adders ...func(*runtime.Scheme) error) (*runtime.Scheme, error) {
	s := runtime.NewScheme()

	if err := clientgoscheme.AddToScheme(s); err != nil {
		return nil, fmt.Errorf("adding client-go scheme: %w", err)
	}

	for _, add := range adders {
		if err := add(s); err != nil {
			return nil, fmt.Errorf("adding adapter scheme: %w", err)
		}
	}

	return s, nil
}

// New connects using kubeconfig, falling back to $KUBECONFIG and then to
// in-cluster config.
func New(kubeconfig string, adders ...func(*runtime.Scheme) error) (*Settings, error) {
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
	dynamicClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating dynamic client: %w", err)
	}
	kubevirt, err := kubevirtclient.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating KubeVirt client: %w", err)
	}

	scheme, err := NewScheme(adders...)
	if err != nil {
		return nil, err
	}

	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("creating controller-runtime client: %w", err)
	}

	return &Settings{Client: c, Config: cfg, K8s: k8s, Dynamic: dynamicClient, Kubevirt: kubevirt,
		Discovery: k8s.Discovery(), Kubeconfig: kubeconfig}, nil
}
