// Package cpu registers the CPU DRA driver adapter.
package cpu

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/johnahull/k8s-dra-harness/internal/driver"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

type adapter struct{}

func init() { driver.Register(adapter{}) }

func (adapter) Name() string        { return "cpu" }
func (adapter) DriverName() string  { return "dra.cpu" }
func (adapter) DeviceClass() string { return "dra.cpu" }
func (adapter) ChartDir() string    { return "deployment/helm/dra-driver-cpu" }
func (adapter) Image(registry, tag string) string {
	return registry + "/dra-driver-cpu/dra-driver-cpu:" + tag
}
func (adapter) Workload() corev1.Container {
	resources := corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1"), corev1.ResourceMemory: resource.MustParse("64Mi")}
	return corev1.Container{Name: "test", Image: "registry.k8s.io/e2e-test-images/busybox:1.29-4", Command: []string{"/bin/sh", "-ec", "env | grep '^DRA_CPUSET_' && echo PASS"}, Resources: corev1.ResourceRequirements{Requests: resources.DeepCopy(), Limits: resources}}
}
func (adapter) Values(values map[string]any, image string) (map[string]any, error) {
	out, err := driver.ImageValues(values, image)
	if err != nil {
		return nil, err
	}
	config, _ := out["driverConfig"].(map[string]any)
	if config == nil {
		config = map[string]any{}
	}
	if _, ok := config["cpuDeviceMode"]; !ok {
		config["cpuDeviceMode"] = "individual"
	}
	out["driverConfig"] = config
	return out, nil
}
func (adapter) Build(ctx context.Context, checkout, image string) error {
	repo, tag, err := driver.SplitImage(image)
	if err != nil {
		return err
	}
	const suffix = "/dra-driver-cpu/dra-driver-cpu"
	if !strings.HasSuffix(repo, suffix) {
		return fmt.Errorf("CPU image repository must end in %s", suffix)
	}
	registry := strings.TrimSuffix(repo, suffix)
	if err := driver.Command(ctx, checkout, os.Environ(), "make", "build-image", "REGISTRY="+registry, "TAG="+tag); err != nil {
		return err
	}
	return driver.Command(ctx, checkout, os.Environ(), "docker", "push", image)
}
