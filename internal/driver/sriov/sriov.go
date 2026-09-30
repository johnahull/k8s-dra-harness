// Package sriov registers the SR-IOV DRA driver adapter.
package sriov

import (
	"context"
	"fmt"

	"github.com/johnahull/k8s-dra-harness/internal/driver"
	corev1 "k8s.io/api/core/v1"
)

const (
	adapterName = "sriov"
	driverName  = "sriovnetwork.k8snetworkplumbingwg.io"
)

type adapter struct{}

func init() { driver.Register(adapter{}) }

func (adapter) Name() string        { return adapterName }
func (adapter) DriverName() string  { return driverName }
func (adapter) DeviceClass() string { return driverName }
func (adapter) ChartDir() string    { return "deployments/helm/dra-driver-sriov" }
func (adapter) Image(registry, tag string) string {
	return registry + "/dra-driver-sriov:" + tag
}

func (adapter) Workload() corev1.Container {
	return corev1.Container{
		Name:    "test",
		Image:   "registry.k8s.io/e2e-test-images/busybox:1.29-4",
		Command: []string{"/bin/sh", "-ec", "env | grep '^SRIOVNETWORK_VF_DEVICE_' && echo PASS"},
	}
}

func (adapter) Values(values map[string]any, image string) (map[string]any, error) {
	out, err := driver.ImageValues(values, image)
	if err != nil {
		return nil, err
	}
	plugin, _ := out["kubeletPlugin"].(map[string]any)
	if plugin == nil {
		plugin = map[string]any{}
	}
	// The generic harness workload does not create a NetworkAttachmentDefinition.
	// MULTUS mode still exercises DRA allocation and CDI injection without asking
	// the driver to fetch standalone-mode network configuration. Users testing
	// SR-IOV networking can explicitly override this with STANDALONE.
	if _, ok := plugin["configurationMode"]; !ok {
		plugin["configurationMode"] = "MULTUS"
	}
	out["kubeletPlugin"] = plugin
	return out, nil
}

func (adapter) Build(ctx context.Context, checkout, image string) error {
	repo, tag, err := driver.SplitImage(image)
	if err != nil {
		return err
	}
	return driver.Command(ctx, checkout, nil, "make", "-f", "deployments/container/Makefile",
		"centos9", "push", fmt.Sprintf("IMAGE_NAME=%s", repo), fmt.Sprintf("VERSION=%s", tag))
}
