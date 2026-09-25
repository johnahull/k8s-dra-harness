package nvidiaoperator

import (
	"strings"
	"testing"

	"github.com/johnahull/k8s-dra-harness/internal/runconfig"
)

func TestChartValuesDisablesClassicPluginForStandaloneDRA(t *testing.T) {
	o := &runconfig.NVIDIAOperator{Image: "nvcr.io/nvidia/gpu-operator:v1.0.0", Values: map[string]any{
		"operator": map[string]any{"resources": map[string]any{"limits": map[string]any{"cpu": "1"}}},
	}}
	values, err := ChartValues(o, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := values["devicePlugin"].(map[string]any)["enabled"]; got != false {
		t.Fatalf("device plugin = %v", got)
	}
	if got := values["platform"].(map[string]any)["openshift"]; got != false {
		t.Fatalf("OpenShift setting = %v", got)
	}
	operator := values["operator"].(map[string]any)
	if operator["repository"] != "nvcr.io/nvidia" || operator["image"] != "gpu-operator" || operator["version"] != "v1.0.0" {
		t.Fatalf("operator image values = %v", operator)
	}
	if o.Values["devicePlugin"] != nil {
		t.Fatal("ChartValues changed caller config")
	}
}

func TestChartValuesRejectsGPUClusterMode(t *testing.T) {
	o := &runconfig.NVIDIAOperator{Values: map[string]any{"gpuCluster": map[string]any{"deployCR": true}}}
	_, err := ChartValues(o, false, false)
	if err == nil || !strings.Contains(err.Error(), "gpuCluster.deployCR") {
		t.Fatalf("ChartValues() = %v", err)
	}
}

func TestDriverValuesFollowsOperatorDriverRoot(t *testing.T) {
	o := &runconfig.NVIDIAOperator{Values: map[string]any{"hostPaths": map[string]any{"driverInstallDir": "/opt/nvidia/driver"}}}
	values, err := DriverValues(map[string]any{}, o)
	if err != nil {
		t.Fatal(err)
	}
	if values["nvidiaDriverRoot"] != "/opt/nvidia/driver" {
		t.Fatalf("nvidiaDriverRoot = %v", values["nvidiaDriverRoot"])
	}
}

func TestDriverValuesRejectsConflictingRoot(t *testing.T) {
	o := &runconfig.NVIDIAOperator{}
	_, err := DriverValues(map[string]any{"nvidiaDriverRoot": "/"}, o)
	if err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("DriverValues() = %v", err)
	}
}

func TestDriverValuesDoesNotOverridePreinstalledDriverRoot(t *testing.T) {
	o := &runconfig.NVIDIAOperator{Values: map[string]any{"driver": map[string]any{"enabled": false}}}
	values, err := DriverValues(map[string]any{"nvidiaDriverRoot": "/"}, o)
	if err != nil {
		t.Fatal(err)
	}
	if values["nvidiaDriverRoot"] != "/" {
		t.Fatalf("nvidiaDriverRoot = %v", values["nvidiaDriverRoot"])
	}
}

func TestGPUResourcesEnabledDefaultsAndOverrides(t *testing.T) {
	if !GPUResourcesEnabled(nil) {
		t.Fatal("GPU resources should default to enabled")
	}
	if GPUResourcesEnabled(map[string]any{"resources": map[string]any{"gpus": map[string]any{"enabled": false}}}) {
		t.Fatal("GPU resources should be disabled")
	}
}

func TestHasStandaloneDriverOnlyCoversGPUResources(t *testing.T) {
	if HasStandaloneDriver([]runconfig.Driver{{Name: "nvidia", Values: map[string]any{"resources": map[string]any{"gpus": map[string]any{"enabled": false}}}}}) {
		t.Fatal("GPU DRA should be disabled when GPU resources are disabled")
	}
	if !HasStandaloneDriver([]runconfig.Driver{{Name: "nvidia"}}) {
		t.Fatal("GPU DRA should default to enabled")
	}
}

func TestWorkloadPodRequestsNVIDIAGPU(t *testing.T) {
	pod := WorkloadPod("test", "gpu")
	gpu := pod.Spec.Containers[0].Resources.Limits["nvidia.com/gpu"]
	if gpu.Value() != 1 {
		t.Fatalf("GPU limit = %s", gpu.String())
	}
}
