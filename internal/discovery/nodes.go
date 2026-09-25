// Package discovery finds AMD GPU nodes and the devices DRA publishes for them.
package discovery

import (
	"context"
	"fmt"
	"regexp"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Labels set by the NodeFeatureRule from AMD's install docs.
const (
	LabelAMDGPU  = "feature.node.kubernetes.io/amd-gpu"
	LabelAMDVGPU = "feature.node.kubernetes.io/amd-vgpu"
	labelTrue    = "true"
)

// nfdAMDPCILabel matches raw NFD PCI labels for AMD (vendor 1002), in either
// the default "<class>_<vendor>" or AMD's "<vendor>_<device>" field layout.
var nfdAMDPCILabel = regexp.MustCompile(`^feature\.node\.kubernetes\.io/pci-([0-9a-f]{4}_)?1002(_[0-9a-f]{4})?\.present$`)

// IsAMDGPUNode reports whether node labels show an AMD GPU.
func IsAMDGPUNode(labels map[string]string) bool {
	if labels[LabelAMDGPU] == labelTrue || labels[LabelAMDVGPU] == labelTrue {
		return true
	}

	for k, v := range labels {
		if v == labelTrue && nfdAMDPCILabel.MatchString(k) {
			return true
		}
	}

	return false
}

// GPUNodes returns every node with an AMD GPU.
func GPUNodes(ctx context.Context, c client.Reader) ([]corev1.Node, error) {
	var nodes corev1.NodeList
	if err := c.List(ctx, &nodes); err != nil {
		return nil, fmt.Errorf("listing nodes: %w", err)
	}

	var gpuNodes []corev1.Node

	for _, n := range nodes.Items {
		if IsAMDGPUNode(n.Labels) {
			gpuNodes = append(gpuNodes, n)
		}
	}

	return gpuNodes, nil
}
