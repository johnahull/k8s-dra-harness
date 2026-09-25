package nvidia

import (
	"reflect"
	"testing"
)

func TestValuesDefaultsToGPUOnly(t *testing.T) {
	input := map[string]any{"resources": map[string]any{"gpus": map[string]any{"enabled": false}}}
	values, err := (adapter{}).Values(input, "quay.io/example/dra-driver:dev")
	if err != nil {
		t.Fatal(err)
	}
	resources := values["resources"].(map[string]any)
	if !reflect.DeepEqual(resources["gpus"], map[string]any{"enabled": false}) {
		t.Fatalf("GPU settings changed: %v", resources["gpus"])
	}
	if !reflect.DeepEqual(resources["computeDomains"], map[string]any{"enabled": false}) {
		t.Fatalf("ComputeDomain defaults = %v", resources["computeDomains"])
	}
	if _, ok := values["gpuResourcesEnabledOverride"]; ok {
		t.Fatal("GPU resource override set while GPU resources are disabled")
	}
	if _, ok := input["resources"].(map[string]any)["computeDomains"]; ok {
		t.Fatal("Values changed caller config")
	}
}

func TestValuesEnablesGPUResourceOverrideByDefault(t *testing.T) {
	values, err := (adapter{}).Values(nil, "quay.io/example/dra-driver:dev")
	if err != nil {
		t.Fatal(err)
	}
	if values["gpuResourcesEnabledOverride"] != true {
		t.Fatalf("GPU resource override = %v", values["gpuResourcesEnabledOverride"])
	}
}

func TestImageAndWorkload(t *testing.T) {
	a := adapter{}
	if got := a.Image("quay.io/team", "run"); got != "quay.io/team/dra-driver-nvidia/dra-driver-nvidia-gpu:run" {
		t.Fatalf("image = %q", got)
	}
	if got := a.Workload().Command; len(got) != 3 || got[2] != "nvidia-smi -L && echo PASS" {
		t.Fatalf("workload command = %v", got)
	}
}

func TestDeviceClassesFollowChartFeatures(t *testing.T) {
	a := adapter{}
	got := a.DeviceClasses(map[string]any{"resources": map[string]any{
		"gpus":           map[string]any{"enabled": true},
		"computeDomains": map[string]any{"enabled": true},
	}})
	want := []string{"gpu.nvidia.com", "mig.nvidia.com", "vfio.gpu.nvidia.com", "compute-domain-daemon.nvidia.com", "compute-domain-default-channel.nvidia.com"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DeviceClasses() = %v, want %v", got, want)
	}
}
