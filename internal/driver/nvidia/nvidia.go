// Package nvidia registers the NVIDIA GPU DRA driver adapter.
package nvidia

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/johnahull/k8s-dra-harness/internal/driver"
	corev1 "k8s.io/api/core/v1"
)

type adapter struct{}

func init() { driver.Register(adapter{}) }

func (adapter) Name() string        { return "nvidia" }
func (adapter) DriverName() string  { return "gpu.nvidia.com" }
func (adapter) DeviceClass() string { return "gpu.nvidia.com" }
func (adapter) ChartDir() string    { return "deployments/helm/dra-driver-nvidia-gpu" }
func (adapter) DeviceClasses(values map[string]any) []string {
	names := []string{}
	resources, _ := values["resources"].(map[string]any)
	gpusEnabled := true
	if gpus, ok := resources["gpus"].(map[string]any); ok {
		if enabled, ok := gpus["enabled"].(bool); ok {
			gpusEnabled = enabled
		}
	}
	if gpusEnabled {
		names = append(names, "gpu.nvidia.com", "mig.nvidia.com", "vfio.gpu.nvidia.com")
	}
	if domains, ok := resources["computeDomains"].(map[string]any); ok {
		if enabled, ok := domains["enabled"].(bool); ok && enabled {
			names = append(names, "compute-domain-daemon.nvidia.com", "compute-domain-default-channel.nvidia.com")
		}
	}
	return names
}
func (adapter) Image(registry, tag string) string {
	return registry + "/dra-driver-nvidia/dra-driver-nvidia-gpu:" + tag
}

func (adapter) Workload() corev1.Container {
	return corev1.Container{
		Name:    "test",
		Image:   "nvcr.io/nvidia/cuda:12.8.1-base-ubuntu22.04",
		Command: []string{"/bin/sh", "-ec", "nvidia-smi -L && echo PASS"},
	}
}

func (adapter) Values(values map[string]any, image string) (map[string]any, error) {
	out, err := driver.ImageValues(values, image)
	if err != nil {
		return nil, err
	}
	resources, _ := out["resources"].(map[string]any)
	if resources == nil {
		resources = map[string]any{}
	}
	gpus, ok := resources["gpus"].(map[string]any)
	if !ok {
		gpus = map[string]any{"enabled": true}
		resources["gpus"] = gpus
	}
	if enabled, ok := gpus["enabled"].(bool); !ok || enabled {
		// The chart requires this explicit opt-in while DRA extended-resource
		// support is not GA, and the operator policy disables the legacy plugin.
		out["gpuResourcesEnabledOverride"] = true
	}
	if _, ok := resources["computeDomains"]; !ok {
		resources["computeDomains"] = map[string]any{"enabled": false}
	}
	out["resources"] = resources
	return out, nil
}

func (adapter) Build(ctx context.Context, checkout, image string) error {
	repo, tag, err := driver.SplitImage(image)
	if err != nil {
		return err
	}
	env := os.Environ()
	makefile := filepath.Join("deployments", "container", "Makefile")
	if err := driver.Command(ctx, checkout, env, "make", "-f", makefile, "build",
		fmt.Sprintf("IMAGE_NAME=%s", repo), fmt.Sprintf("VERSION=%s", tag)); err != nil {
		return err
	}
	return driver.Command(ctx, checkout, env, "docker", "push", image)
}
