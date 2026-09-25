package discovery

import (
	"context"
	"fmt"

	resourcev1 "k8s.io/api/resource/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DRADriverName is the AMD DRA driver name and its DeviceClass name.
const DRADriverName = "gpu.amd.com"

// fullGPUType is the "type" attribute value of a whole (unpartitioned) GPU.
const fullGPUType = "amdgpu"

// DevicesByNode counts full AMD GPUs per node from the newest generation of
// each ResourceSlice pool. It returns a NoKindMatch error (check with
// meta.IsNoMatchError) when resource.k8s.io/v1 is not served.
func DevicesByNode(ctx context.Context, c client.Reader) (map[string]int, error) {
	var slices resourcev1.ResourceSliceList
	if err := c.List(ctx, &slices); err != nil {
		return nil, fmt.Errorf("listing ResourceSlices: %w", err)
	}

	latest := map[string]int64{}

	for _, s := range slices.Items {
		if s.Spec.Driver == DRADriverName && s.Spec.Pool.Generation > latest[s.Spec.Pool.Name] {
			latest[s.Spec.Pool.Name] = s.Spec.Pool.Generation
		}
	}

	counts := map[string]int{}

	for _, s := range slices.Items {
		if s.Spec.Driver != DRADriverName || s.Spec.NodeName == nil ||
			s.Spec.Pool.Generation != latest[s.Spec.Pool.Name] {
			continue
		}

		for _, d := range s.Spec.Devices {
			if deviceType(d) == fullGPUType {
				counts[*s.Spec.NodeName]++
			}
		}
	}

	return counts, nil
}

// deviceType returns the device's "type" attribute. Unqualified attribute
// names belong to the driver's domain, so both spellings are accepted.
func deviceType(d resourcev1.Device) string {
	for _, key := range []resourcev1.QualifiedName{"type", DRADriverName + "/type"} {
		if a, ok := d.Attributes[key]; ok && a.StringValue != nil {
			return *a.StringValue
		}
	}

	return ""
}
