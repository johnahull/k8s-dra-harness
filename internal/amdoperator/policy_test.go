package amdoperator

import (
	"testing"

	"github.com/johnahull/k8s-dra-harness/internal/runconfig"
)

func TestChartValuesPreservesUserConfig(t *testing.T) {
	o := &runconfig.AMDOperator{Image: "quay.io/team/operator:dev", Values: map[string]any{
		"deviceConfig":      map[string]any{"spec": map[string]any{"devicePlugin": map[string]any{"enableDevicePlugin": true}}},
		"controllerManager": map[string]any{"manager": map[string]any{"replicas": 2}},
	}}
	values, err := ChartValues(o, true)
	if err != nil {
		t.Fatal(err)
	}
	configured := values["deviceConfig"].(map[string]any)["spec"].(map[string]any)["devicePlugin"].(map[string]any)["enableDevicePlugin"]
	if configured != false {
		t.Fatalf("device plugin override = %v", configured)
	}
	original := o.Values["deviceConfig"].(map[string]any)["spec"].(map[string]any)["devicePlugin"].(map[string]any)["enableDevicePlugin"]
	if original != true {
		t.Fatal("operator override changed caller config")
	}
	manager := values["controllerManager"].(map[string]any)["manager"].(map[string]any)
	image := manager["image"].(map[string]any)
	if image["repository"] != "quay.io/team/operator" || image["tag"] != "dev" || manager["replicas"] != 2 {
		t.Fatalf("manager values = %v", manager)
	}
}

func TestDeviceConfigSpecDisablesManagedDriver(t *testing.T) {
	input := map[string]any{"driver": map[string]any{"enable": true}}
	spec := DeviceConfigSpec(input, true)
	if spec["driver"].(map[string]any)["enable"] != true || spec["draDriver"].(map[string]any)["enable"] != false {
		t.Fatalf("DeviceConfig spec = %v", spec)
	}
	if _, ok := input["draDriver"]; ok {
		t.Fatal("DeviceConfigSpec changed caller config")
	}
}

func TestWorkloadPodRequestsAMDDevice(t *testing.T) {
	pod := WorkloadPod("test", "gpu")
	if pod.Namespace != "test" || pod.Name != "gpu" {
		t.Fatalf("workload identity = %s/%s", pod.Namespace, pod.Name)
	}
	gpu := pod.Spec.Containers[0].Resources.Limits["amd.com/gpu"]
	if gpu.Value() != 1 {
		t.Fatalf("GPU limit = %s", gpu.String())
	}
}
