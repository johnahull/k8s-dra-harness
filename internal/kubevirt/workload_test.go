package kubevirt

import (
	"encoding/json"
	"testing"

	"github.com/johnahull/k8s-dra-harness/internal/runconfig"
	corev1 "k8s.io/api/core/v1"
)

func TestBuildVMIGPU(t *testing.T) {
	vmi := BuildVMI("test-vmi", "test", &runconfig.KubeVirt{Image: "example/image"}, "gpu", "nvidia-gpu", "nvidia", "claim", map[string]string{"test": "true"})
	if got := len(vmi.Spec.Domain.Devices.GPUs); got != 1 {
		t.Fatalf("GPU count = %d, want 1", got)
	}
	gpu := vmi.Spec.Domain.Devices.GPUs[0]
	if gpu.Name != "nvidia-gpu" || gpu.ClaimRequest == nil || gpu.ClaimName != "nvidia" || gpu.RequestName != requestName {
		t.Fatalf("unexpected GPU mapping: %+v", gpu)
	}
	if len(vmi.Spec.Domain.Devices.HostDevices) != 0 || len(vmi.Spec.ResourceClaims) != 1 || vmi.Spec.ResourceClaims[0].ResourceClaimName == nil || *vmi.Spec.ResourceClaims[0].ResourceClaimName != "claim" {
		t.Fatalf("unexpected VMI claims: %+v", vmi.Spec.ResourceClaims)
	}
}

func TestBuildVMIHostDeviceAndCloudInit(t *testing.T) {
	vmi := BuildVMI("test-vmi", "test", &runconfig.KubeVirt{Image: "example/image", CloudInitSecret: "guest-init"}, "hostDevice", "amd-gpu", "amd", "claim", nil)
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

func TestBuildVFIOClaim(t *testing.T) {
	claim, err := BuildClaim("claim", "test", &runconfig.KubeVirt{
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
