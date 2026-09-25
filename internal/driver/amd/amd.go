// Package amd registers the AMD GPU DRA driver adapter.
package amd

import (
	"context"
	"os"
	"strings"

	"github.com/johnahull/k8s-dra-harness/internal/driver"
	corev1 "k8s.io/api/core/v1"
)

type adapter struct{}

func init() {
	driver.Register(adapter{})
	driver.RegisterJoint("amd", "cpu", jointWorkload)
}

func (adapter) Name() string                      { return "amd" }
func (adapter) DriverName() string                { return "gpu.amd.com" }
func (adapter) DeviceClass() string               { return "gpu.amd.com" }
func (adapter) ChartDir() string                  { return "helm-charts-k8s" }
func (adapter) Image(registry, tag string) string { return registry + "/k8s-gpu-dra-driver:" + tag }
func (adapter) Workload() corev1.Container {
	return corev1.Container{Name: "test", Image: "docker.io/rocm/dev-ubuntu-22.04:6.4", Command: []string{"/bin/sh", "-ec", "rocm-smi && echo PASS"}}
}
func (adapter) Values(values map[string]any, image string) (map[string]any, error) {
	return driver.ImageValues(values, image)
}
func (adapter) Build(ctx context.Context, checkout, image string) error {
	repo, tag, err := driver.SplitImage(image)
	if err != nil {
		return err
	}
	base := repo[strings.LastIndex(repo, "/")+1:]
	registry := strings.TrimSuffix(repo, "/"+base)
	env := append(os.Environ(), "DRIVER_IMAGE_REGISTRY="+registry, "DRIVER_IMAGE_NAME="+base, "DRIVER_IMAGE_TAG="+tag)
	if err := driver.Command(ctx, checkout, env, "make", "build"); err != nil {
		return err
	}
	return driver.Command(ctx, checkout, env, "docker", "push", image)
}

func jointWorkload(adapters []driver.Adapter) corev1.Container {
	container := (adapter{}).Workload()
	container.Command = []string{"/bin/sh", "-ec", "rocm-smi && env | grep '^DRA_CPUSET_' && echo PASS"}
	for _, selected := range adapters {
		if selected.Name() == "cpu" {
			container.Resources = selected.Workload().Resources
		}
	}
	return container
}
