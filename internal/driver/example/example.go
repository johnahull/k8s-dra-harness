// Package example registers the Kubernetes mock-device DRA example adapter.
package example

import (
	"context"
	"fmt"
	"os"

	"github.com/johnahull/k8s-dra-harness/internal/driver"
	corev1 "k8s.io/api/core/v1"
)

type adapter struct{}

func init() { driver.Register(adapter{}) }

func (adapter) Name() string        { return "example" }
func (adapter) DriverName() string  { return "gpu.example.com" }
func (adapter) DeviceClass() string { return "gpu.example.com" }
func (adapter) ChartDir() string    { return "deployments/helm/dra-example-driver" }
func (adapter) Image(registry, tag string) string {
	return registry + "/dra-example-driver/dra-example-driver:" + tag
}

func (adapter) Workload() corev1.Container {
	return corev1.Container{
		Name:    "test",
		Image:   "registry.k8s.io/e2e-test-images/busybox:1.29-4",
		Command: []string{"/bin/sh", "-ec", "env | grep '^GPU_DEVICE_' && echo PASS"},
	}
}

func (adapter) Values(values map[string]any, image string) (map[string]any, error) {
	out, err := driver.ImageValues(values, image)
	if err != nil {
		return nil, err
	}
	if _, ok := out["deviceProfile"]; !ok {
		out["deviceProfile"] = "gpu"
	}
	return out, nil
}

func (adapter) Build(ctx context.Context, checkout, image string) error {
	repo, tag, err := driver.SplitImage(image)
	if err != nil {
		return err
	}
	env := append(os.Environ(), "CONTAINER_TOOL=docker")
	return driver.Command(ctx, checkout, env, "make", "-f", "deployments/container/Makefile", "push", fmt.Sprintf("IMAGE_NAME=%s", repo), fmt.Sprintf("VERSION=%s", tag))
}
