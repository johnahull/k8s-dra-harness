package kubevirt

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/johnahull/k8s-dra-harness/internal/runconfig"
	corev1 "k8s.io/api/core/v1"
)

const (
	testClaimName = "claim"
)

func TestBuildVMIGPU(t *testing.T) {
	vmi, err := BuildVMI("test-vmi", "test", &runconfig.KubeVirt{Image: "example/image"}, "gpu", "nvidia-gpu", "nvidia", testClaimName, map[string]string{"test": "true"})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(vmi.Spec.Domain.Devices.GPUs); got != 1 {
		t.Fatalf("GPU count = %d, want 1", got)
	}
	gpu := vmi.Spec.Domain.Devices.GPUs[0]
	if gpu.Name != "nvidia-gpu" || gpu.ClaimRequest == nil || gpu.ClaimName != "nvidia" || gpu.RequestName != requestName {
		t.Fatalf("unexpected GPU mapping: %+v", gpu)
	}
	if len(vmi.Spec.Domain.Devices.HostDevices) != 0 || len(vmi.Spec.ResourceClaims) != 1 || vmi.Spec.ResourceClaims[0].ResourceClaimName == nil || *vmi.Spec.ResourceClaims[0].ResourceClaimName != testClaimName {
		t.Fatalf("unexpected VMI claims: %+v", vmi.Spec.ResourceClaims)
	}
}

func TestBuildVMIMultiGPU(t *testing.T) {
	vmi, err := BuildVMI("test-vmi", "test", &runconfig.KubeVirt{Image: "example/image", DeviceCount: 2}, "gpu", "amd", testClaimName, "claim-object", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(vmi.Spec.Domain.Devices.GPUs); got != 2 {
		t.Fatalf("GPU count = %d, want 2", got)
	}
	for i, gpu := range vmi.Spec.Domain.Devices.GPUs {
		wantName := fmt.Sprintf("amd-%d", i)
		wantRequest := fmt.Sprintf("device-%d", i)
		if gpu.Name != wantName || gpu.ClaimRequest == nil || gpu.ClaimName != testClaimName || gpu.RequestName != wantRequest {
			t.Fatalf("GPU[%d] mapping = %+v, want name %q and request %q", i, gpu, wantName, wantRequest)
		}
	}
	if len(vmi.Spec.ResourceClaims) != 1 || vmi.Spec.ResourceClaims[0].ResourceClaimName == nil || *vmi.Spec.ResourceClaims[0].ResourceClaimName != "claim-object" {
		t.Fatalf("unexpected VMI claims: %+v", vmi.Spec.ResourceClaims)
	}
}

func TestBuildVMIMultiHostDevice(t *testing.T) {
	vmi, err := BuildVMI("test-vmi", "test", &runconfig.KubeVirt{Image: "example/image", DeviceCount: 2}, "hostDevice", "amd", testClaimName, "claim-object", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(vmi.Spec.Domain.Devices.HostDevices); got != 2 {
		t.Fatalf("HostDevice count = %d, want 2", got)
	}
	for i, hostDevice := range vmi.Spec.Domain.Devices.HostDevices {
		wantName := fmt.Sprintf("amd-%d", i)
		wantRequest := fmt.Sprintf("device-%d", i)
		if hostDevice.Name != wantName || hostDevice.ClaimRequest == nil || hostDevice.ClaimName != testClaimName || hostDevice.RequestName != wantRequest {
			t.Fatalf("HostDevice[%d] mapping = %+v, want name %q and request %q", i, hostDevice, wantName, wantRequest)
		}
	}
}

func TestBuildVMIHostDeviceAndCloudInit(t *testing.T) {
	vmi, err := BuildVMI("test-vmi", "test", &runconfig.KubeVirt{Image: "example/image", CloudInitSecret: "guest-init"}, "hostDevice", "amd-gpu", "amd", "claim", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(vmi.Spec.Domain.Devices.HostDevices) != 1 || vmi.Spec.Domain.Devices.HostDevices[0].ClaimRequest == nil {
		t.Fatalf("unexpected HostDevice mapping: %+v", vmi.Spec.Domain.Devices.HostDevices)
	}
	if len(vmi.Spec.Volumes) != 2 || vmi.Spec.Volumes[1].CloudInitNoCloud == nil || vmi.Spec.Volumes[1].CloudInitNoCloud.UserDataSecretRef.Name != "guest-init" {
		t.Fatalf("unexpected cloud-init volume: %+v", vmi.Spec.Volumes)
	}
	if len(vmi.Spec.Domain.Devices.Disks) != 2 || vmi.Spec.Domain.Devices.Disks[1].CDRom == nil {
		t.Fatalf("unexpected cloud-init disk: %+v", vmi.Spec.Domain.Devices.Disks)
	}
}

func TestBuildVMICPU(t *testing.T) {
	vmi, err := BuildVMI("test-vmi", "test", &runconfig.KubeVirt{Image: "example/image"}, "cpu", cpuRequestName, testClaimName, "claim-object", nil)
	if err != nil {
		t.Fatal(err)
	}
	if vmi.Spec.Domain.CPU == nil || vmi.Spec.Domain.CPU.Cores != 1 || !vmi.Spec.Domain.CPU.DedicatedCPUPlacement {
		t.Fatalf("unexpected CPU configuration: %+v", vmi.Spec.Domain.CPU)
	}
	if len(vmi.Spec.Domain.Devices.GPUs) != 0 || len(vmi.Spec.Domain.Devices.HostDevices) != 0 || len(vmi.Spec.Networks) != 0 {
		t.Fatalf("CPU VMI has unrelated device wiring: devices=%+v networks=%+v", vmi.Spec.Domain.Devices, vmi.Spec.Networks)
	}
	if got := vmi.Annotations["kubevirt.io/dra-manual-claim"]; got != testClaimName {
		t.Fatalf("manual claim annotation = %q, want claim", got)
	}
	if got := vmi.Spec.ResourceClaims[0].ResourceClaimName; got == nil || *got != "claim-object" {
		t.Fatalf("unexpected CPU resource claim: %+v", vmi.Spec.ResourceClaims)
	}
}

func TestBuildVMISRIOVNetwork(t *testing.T) {
	vmi, err := BuildVMI("test-vmi", "test", &runconfig.KubeVirt{Image: "example/image"}, "network", "sriov", testClaimName, "claim-object", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(vmi.Spec.Networks) != 2 || len(vmi.Spec.Domain.Devices.Interfaces) != 2 {
		t.Fatalf("unexpected SR-IOV network wiring: networks=%+v interfaces=%+v", vmi.Spec.Networks, vmi.Spec.Domain.Devices.Interfaces)
	}
	if vmi.Spec.Networks[1].ResourceClaim == nil || vmi.Spec.Networks[1].ResourceClaim.ClaimName != testClaimName || vmi.Spec.Networks[1].ResourceClaim.RequestName != requestName {
		t.Fatalf("unexpected SR-IOV network claim: %+v", vmi.Spec.Networks[1])
	}
	if vmi.Spec.Domain.Devices.Interfaces[1].SRIOV == nil {
		t.Fatalf("SR-IOV interface binding is missing: %+v", vmi.Spec.Domain.Devices.Interfaces[1])
	}
}

func TestBuildClaimMultiDevice(t *testing.T) {
	claim, err := BuildClaim(testClaimName, "test", &runconfig.KubeVirt{DeviceCount: 2, Selector: `device.attributes["gpu.amd.com"].type == "vfio"`}, "amd", "gpu.amd.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(claim.Spec.Devices.Requests); got != 2 {
		t.Fatalf("request count = %d, want 2", got)
	}
	for i, request := range claim.Spec.Devices.Requests {
		want := fmt.Sprintf("device-%d", i)
		if request.Name != want || request.Exactly == nil || len(request.Exactly.Selectors) != 1 {
			t.Fatalf("request[%d] = %+v, want name %q and selector", i, request, want)
		}
	}
}

func TestBuildVMIRejectsMultiDeviceCPU(t *testing.T) {
	if _, err := BuildVMI("test-vmi", "test", &runconfig.KubeVirt{Image: "example/image", DeviceCount: 2}, "cpu", cpuRequestName, testClaimName, "claim-object", nil); err == nil {
		t.Fatal("BuildVMI accepted multi-device CPU workload")
	}
}
func TestBuildVMIApertureTestOptions(t *testing.T) {
	config := &runconfig.KubeVirt{
		Image:          "example/image",
		CPUModel:       "host-passthrough",
		NetworkBinding: "masquerade",
	}
	vmi, err := BuildVMI("test-vmi", "test", config, runconfig.KubeVirtAttachmentGPU, "amd", "amd", "claim", nil)
	if err != nil {
		t.Fatal(err)
	}
	if vmi.Spec.Domain.CPU == nil || vmi.Spec.Domain.CPU.Model != "host-passthrough" {
		t.Fatalf("CPU model = %+v, want host-passthrough", vmi.Spec.Domain.CPU)
	}
	if len(vmi.Spec.Networks) != 1 || vmi.Spec.Networks[0].Pod == nil {
		t.Fatalf("unexpected networks: %+v", vmi.Spec.Networks)
	}
	if len(vmi.Spec.Domain.Devices.Interfaces) != 1 || vmi.Spec.Domain.Devices.Interfaces[0].Masquerade == nil {
		t.Fatalf("unexpected interfaces: %+v", vmi.Spec.Domain.Devices.Interfaces)
	}
}

func TestBuildVFIOClaim(t *testing.T) {
	claim, err := BuildClaim(testClaimName, "test", &runconfig.KubeVirt{
		Selector: `device.attributes["gpu.amd.com"].type == "vfio"`,
		ClaimConfig: &runconfig.KubeVirtClaim{
			Driver: "gpu.amd.com",
			Parameters: map[string]any{
				"apiVersion": "gpu.resource.amd.com/v1alpha1",
				"kind":       "VfioDeviceConfig",
				"iommu":      map[string]any{"backendPolicy": "LegacyOnly"},
			},
		},
	}, "amd", "gpu.amd.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	request := claim.Spec.Devices.Requests[0]
	if len(request.Exactly.Selectors) != 1 || request.Exactly.Selectors[0].CEL == nil || request.Exactly.Selectors[0].CEL.Expression != `device.attributes["gpu.amd.com"].type == "vfio"` {
		t.Fatalf("unexpected claim selector: %+v", request.Exactly.Selectors)
	}
	if len(claim.Spec.Devices.Config) != 1 || claim.Spec.Devices.Config[0].Opaque == nil {
		t.Fatalf("unexpected claim config: %+v", claim.Spec.Devices.Config)
	}
	opaque := claim.Spec.Devices.Config[0].Opaque
	if opaque.Driver != "gpu.amd.com" {
		t.Fatalf("opaque driver = %q", opaque.Driver)
	}
	var parameters map[string]any
	if err := json.Unmarshal(opaque.Parameters.Raw, &parameters); err != nil {
		t.Fatal(err)
	}
	if parameters["kind"] != "VfioDeviceConfig" {
		t.Fatalf("unexpected opaque parameters: %v", parameters)
	}
}

func TestBuildCPUClaim(t *testing.T) {
	claim, err := BuildClaim(testClaimName, "test", &runconfig.KubeVirt{}, cpuRequestName, "dra.cpu", nil)
	if err != nil {
		t.Fatal(err)
	}
	request := claim.Spec.Devices.Requests[0]
	if request.Name != cpuRequestName || request.Exactly.Capacity == nil {
		t.Fatalf("unexpected CPU request: %+v", request)
	}
	quantity := request.Exactly.Capacity.Requests["dra.cpu/cpu"]
	if got := quantity.String(); got != "1" {
		t.Fatalf("CPU capacity = %q, want 1", got)
	}
	if len(claim.Spec.Devices.Config) != 0 {
		t.Fatalf("unexpected CPU opaque config: %+v", claim.Spec.Devices.Config)
	}
}

func TestKubeVirtFeatureGate(t *testing.T) {
	tests := []struct {
		attachment string
		gate       string
		defaultOn  bool
	}{
		{attachment: runconfig.KubeVirtAttachmentGPU, gate: "GPUsWithDRA", defaultOn: true},
		{attachment: runconfig.KubeVirtAttachmentHostDevice, gate: "HostDevicesWithDRA", defaultOn: true},
		{attachment: runconfig.KubeVirtAttachmentCPU, gate: "CPUsWithDRA"},
		{attachment: runconfig.KubeVirtAttachmentNetwork, gate: "NetworkDevicesWithDRA"},
	}
	for _, test := range tests {
		gate, defaultOn := kubeVirtFeatureGate(test.attachment)
		if gate != test.gate || defaultOn != test.defaultOn {
			t.Errorf("kubeVirtFeatureGate(%q) = %q, %t; want %q, %t", test.attachment, gate, defaultOn, test.gate, test.defaultOn)
		}
	}
	if gate, defaultOn := kubeVirtFeatureGate("unsupported"); gate != "" || defaultOn {
		t.Fatalf("unsupported attachment mapped to gate %q, defaultOn=%t", gate, defaultOn)
	}
}

func TestPodCarriesClaim(t *testing.T) {
	claim := "claim"
	if !podCarriesClaim(&corev1.Pod{Spec: corev1.PodSpec{ResourceClaims: []corev1.PodResourceClaim{{ResourceClaimName: &claim}}}}, claim) {
		t.Fatal("pod claim was not detected")
	}
	if podCarriesClaim(&corev1.Pod{}, claim) {
		t.Fatal("missing pod claim was detected")
	}
}

func TestNestedStringSlice(t *testing.T) {
	values, found, err := nestedStringSlice(map[string]any{"spec": map[string]any{"featureGates": []any{"GPUsWithDRA"}}}, "spec", "featureGates")
	if err != nil || !found || len(values) != 1 || values[0] != "GPUsWithDRA" {
		t.Fatalf("nestedStringSlice() = %v, %v, %v", values, found, err)
	}
	_, found, err = nestedStringSlice(map[string]any{}, "spec", "featureGates")
	if err != nil || found {
		t.Fatalf("missing nested value = found %v, err %v", found, err)
	}
}
