// Package driver contains the driver-specific build and deployment contract.
package driver

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/johnahull/k8s-dra-harness/internal/runconfig"
)

// Adapter describes the behavior that differs between DRA implementations.
type Adapter interface {
	Name() string
	DriverName() string
	DeviceClass() string
	ChartDir() string
	Build(ctx context.Context, checkout, image string) error
	WorkloadImage() string
	WorkloadCommand() []string
}

const cpuName = "cpu"

// Get returns a registered adapter.
func Get(name string) (Adapter, error) {
	switch name {
	case "amd":
		return amd{}, nil
	case cpuName:
		return cpu{}, nil
	default:
		return nil, fmt.Errorf("unsupported driver %q", name)
	}
}

type amd struct{}

func (amd) Name() string          { return "amd" }
func (amd) DriverName() string    { return "gpu.amd.com" }
func (amd) DeviceClass() string   { return "gpu.amd.com" }
func (amd) ChartDir() string      { return "helm-charts-k8s" }
func (amd) WorkloadImage() string { return "docker.io/rocm/dev-ubuntu-22.04:6.4" }
func (amd) WorkloadCommand() []string {
	return []string{"/bin/sh", "-ec", "rocm-smi && echo PASS"}
}
func (amd) Build(ctx context.Context, checkout, image string) error {
	repo, tag, err := splitImage(image)
	if err != nil {
		return err
	}
	base := repo[strings.LastIndex(repo, "/")+1:]
	registry := strings.TrimSuffix(repo, "/"+base)
	env := append(os.Environ(), "DRIVER_IMAGE_REGISTRY="+registry, "DRIVER_IMAGE_NAME="+base, "DRIVER_IMAGE_TAG="+tag)
	if err := command(ctx, checkout, env, "make", "build"); err != nil {
		return err
	}
	return command(ctx, checkout, env, "docker", "push", image)
}

type cpu struct{}

func (cpu) Name() string          { return cpuName }
func (cpu) DriverName() string    { return "dra.cpu" }
func (cpu) DeviceClass() string   { return "dra.cpu" }
func (cpu) ChartDir() string      { return "deployment/helm/dra-driver-cpu" }
func (cpu) WorkloadImage() string { return "registry.k8s.io/e2e-test-images/busybox:1.29-4" }
func (cpu) WorkloadCommand() []string {
	return []string{"/bin/sh", "-ec", "env | grep '^DRA_CPUSET_' && echo PASS"}
}
func (cpu) Build(ctx context.Context, checkout, image string) error {
	repo, tag, err := splitImage(image)
	if err != nil {
		return err
	}
	const suffix = "/dra-driver-cpu/dra-driver-cpu"
	if !strings.HasSuffix(repo, suffix) {
		return fmt.Errorf("CPU image repository must end in %s", suffix)
	}
	registry := strings.TrimSuffix(repo, suffix)
	if err := command(ctx, checkout, os.Environ(), "make", "build-image", "REGISTRY="+registry, "TAG="+tag); err != nil {
		return err
	}
	return command(ctx, checkout, os.Environ(), "docker", "push", image)
}

// Resolve returns the image and chart to install. A checkout builds and pushes
// its image before the cluster is touched.
func Resolve(ctx context.Context, adapter Adapter, d runconfig.Driver, registry, tag string) (string, string, error) {
	if d.SourcePath == "" {
		return d.Image, d.Chart, nil
	}
	info, err := os.Stat(d.SourcePath)
	if err != nil || !info.IsDir() {
		return "", "", fmt.Errorf("source checkout %q is not a directory", d.SourcePath)
	}
	chart := filepath.Join(d.SourcePath, adapter.ChartDir())
	if d.Chart != "" {
		chart = d.Chart
	}
	if info, err = os.Stat(filepath.Join(chart, "Chart.yaml")); err != nil || info.IsDir() {
		return "", "", fmt.Errorf("chart %q has no Chart.yaml", chart)
	}
	registry = strings.TrimSuffix(registry, "/")
	var image string
	if adapter.Name() == "cpu" {
		image = registry + "/dra-driver-cpu/dra-driver-cpu:" + tag
	} else {
		image = registry + "/k8s-gpu-dra-driver:" + tag
	}
	if err := adapter.Build(ctx, d.SourcePath, image); err != nil {
		return "", "", fmt.Errorf("building %s: %w", d.Name, err)
	}
	return image, chart, nil
}

// SplitImage supports tagged references. Digests are rejected because both
// upstream charts take separate repository and tag values.
func SplitImage(image string) (string, string, error) { return splitImage(image) }

func splitImage(image string) (string, string, error) {
	if strings.Contains(image, "@") {
		return "", "", fmt.Errorf("image digest %q is unsupported by the driver charts", image)
	}
	i := strings.LastIndex(image, ":")
	if i <= strings.LastIndex(image, "/") || i == len(image)-1 {
		return "", "", fmt.Errorf("image %q needs an explicit tag", image)
	}
	return image[:i], image[i+1:], nil
}

func command(ctx context.Context, dir string, env []string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}
