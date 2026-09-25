// Package driver contains the driver-specific build and deployment contract.
package driver

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// Adapter describes the behavior that differs between DRA implementations.
type Adapter interface {
	Name() string
	DriverName() string
	DeviceClass() string
	ChartDir() string
	Image(registry, tag string) string
	Build(ctx context.Context, checkout, image string) error
	Values(values map[string]any, image string) (map[string]any, error)
	Workload() corev1.Container
}

// Source selects a checkout to build or a published image to deploy.
type Source struct {
	Path  string
	Image string
	Chart string
}

var adapters = map[string]Adapter{}

// Register makes an adapter available by name to run configuration.
func Register(adapter Adapter) {
	name := adapter.Name()
	if name == "" {
		panic("driver adapter name is empty")
	}
	if _, exists := adapters[name]; exists {
		panic("duplicate driver adapter " + name)
	}
	adapters[name] = adapter
}

// Get returns a registered adapter.
func Get(name string) (Adapter, error) {
	adapter, ok := adapters[name]
	if !ok {
		return nil, fmt.Errorf("unsupported driver %q", name)
	}
	return adapter, nil
}

// Resolve returns the image and chart to install. A checkout builds and pushes
// its image before the cluster is touched.
func Resolve(ctx context.Context, adapter Adapter, source Source, registry, tag string) (string, string, error) {
	if source.Path == "" {
		return source.Image, source.Chart, nil
	}
	info, err := os.Stat(source.Path)
	if err != nil || !info.IsDir() {
		return "", "", fmt.Errorf("source checkout %q is not a directory", source.Path)
	}
	chart := filepath.Join(source.Path, adapter.ChartDir())
	if source.Chart != "" {
		chart = source.Chart
	}
	if info, err = os.Stat(filepath.Join(chart, "Chart.yaml")); err != nil || info.IsDir() {
		return "", "", fmt.Errorf("chart %q has no Chart.yaml", chart)
	}
	registry = strings.TrimSuffix(registry, "/")
	image := adapter.Image(registry, tag)
	if err := adapter.Build(ctx, source.Path, image); err != nil {
		return "", "", fmt.Errorf("building %s: %w", adapter.Name(), err)
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

func Command(ctx context.Context, dir string, env []string, name string, args ...string) error {
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
