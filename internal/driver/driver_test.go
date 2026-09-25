package driver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/johnahull/k8s-dra-harness/internal/runconfig"
)

func TestSplitImage(t *testing.T) {
	repo, tag, err := SplitImage("localhost:5000/team/cpu:dev")
	if err != nil || repo != "localhost:5000/team/cpu" || tag != "dev" {
		t.Fatalf("SplitImage() = %q, %q, %v", repo, tag, err)
	}
	for _, ref := range []string{"registry/team/cpu", "registry/team/cpu@sha256:abc", "registry/team/cpu:"} {
		if _, _, err := SplitImage(ref); err == nil {
			t.Errorf("expected invalid image %q", ref)
		}
	}
}

func TestResolveRejectsMissingChartBeforeBuild(t *testing.T) {
	dir := t.TempDir()
	_, _, err := Resolve(context.Background(), amd{}, runconfig.Driver{Name: "amd", SourcePath: dir}, "quay.io/team", "run1")
	if err == nil || !strings.Contains(err.Error(), "Chart.yaml") {
		t.Fatalf("Resolve() = %v", err)
	}
	chart := filepath.Join(dir, "helm-charts-k8s")
	if err := os.Mkdir(chart, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(chart, "Chart.yaml"), []byte("name: test\n"), 0600); err != nil {
		t.Fatal(err)
	}
}

type recordingAdapter struct{ image string }

const (
	testAMDName   = "amd"
	testAMDDriver = "gpu.amd.com"
)

func (a *recordingAdapter) Name() string              { return testAMDName }
func (a *recordingAdapter) DriverName() string        { return testAMDDriver }
func (a *recordingAdapter) DeviceClass() string       { return testAMDDriver }
func (a *recordingAdapter) ChartDir() string          { return "chart" }
func (a *recordingAdapter) WorkloadImage() string     { return "unused" }
func (a *recordingAdapter) WorkloadCommand() []string { return nil }
func (a *recordingAdapter) Build(_ context.Context, _, image string) error {
	a.image = image
	return nil
}

func TestResolveBuildsCheckoutImageAndChart(t *testing.T) {
	dir := t.TempDir()
	chart := filepath.Join(dir, "chart")
	if err := os.Mkdir(chart, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(chart, "Chart.yaml"), []byte("name: test\n"), 0600); err != nil {
		t.Fatal(err)
	}
	a := &recordingAdapter{}
	image, gotChart, err := Resolve(context.Background(), a, runconfig.Driver{Name: testAMDName, SourcePath: dir}, "quay.io/team/", "run123")
	if err != nil {
		t.Fatal(err)
	}
	if image != "quay.io/team/k8s-gpu-dra-driver:run123" || a.image != image || gotChart != chart {
		t.Fatalf("Resolve() = image %q, built %q, chart %q", image, a.image, gotChart)
	}
}
