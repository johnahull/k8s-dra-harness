package discovery

import (
	"context"
	"reflect"
	"testing"

	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func device(name, typ string) resourcev1.Device {
	return resourcev1.Device{
		Name:       name,
		Attributes: map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{"type": {StringValue: ptr.To(typ)}},
	}
}

func slice(name, driver, node, pool string, generation int64, devices ...resourcev1.Device) *resourcev1.ResourceSlice {
	return &resourcev1.ResourceSlice{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: resourcev1.ResourceSliceSpec{
			Driver:   driver,
			NodeName: ptr.To(node),
			Pool:     resourcev1.ResourcePool{Name: pool, Generation: generation, ResourceSliceCount: 1},
			Devices:  devices,
		},
	}
}

func TestDevicesByNode(t *testing.T) {
	c := newFakeClient(t,
		slice("n1-gen1", DRADriverName, "node-1", "node-1", 1, device("gpu-0", "amdgpu")),
		slice("n1-gen2", DRADriverName, "node-1", "node-1", 2,
			device("gpu-0", "amdgpu"), device("gpu-1", "amdgpu"), device("gpu-1-p0", "amdgpu-partition")),
		slice("n2", DRADriverName, "node-2", "node-2", 1, device("gpu-0", "amdgpu")),
		slice("nv", "gpu.nvidia.com", "node-3", "node-3", 1, device("gpu-0", "amdgpu")),
	)

	got, err := DevicesByNode(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]int{"node-1": 2, "node-2": 1}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DevicesByNode = %v, want %v (stale gen, partitions and other drivers excluded)", got, want)
	}
}
