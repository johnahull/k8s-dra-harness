package discovery

import (
	"context"
	"testing"

	"github.com/johnahull/amd-gpu-e2e/pkg/clients"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newFakeClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()

	s, err := clients.NewScheme()
	if err != nil {
		t.Fatal(err)
	}

	return fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()
}

func TestIsAMDGPUNode(t *testing.T) {
	tests := []struct {
		labels map[string]string
		want   bool
	}{
		{map[string]string{LabelAMDGPU: "true"}, true},
		{map[string]string{LabelAMDVGPU: "true"}, true},
		{map[string]string{LabelAMDGPU: "false"}, false},
		{map[string]string{"feature.node.kubernetes.io/pci-0380_1002.present": "true"}, true},
		{map[string]string{"feature.node.kubernetes.io/pci-1002_74a1.present": "true"}, true},
		{map[string]string{"feature.node.kubernetes.io/pci-0300_10de.present": "true"}, false}, // NVIDIA
		{map[string]string{"feature.node.kubernetes.io/pci-0300_1a03.present": "true"}, false}, // ASPEED BMC
		{map[string]string{"node-role.kubernetes.io/worker": ""}, false},
	}

	for _, tt := range tests {
		if got := IsAMDGPUNode(tt.labels); got != tt.want {
			t.Errorf("IsAMDGPUNode(%v) = %v, want %v", tt.labels, got, tt.want)
		}
	}
}

func TestGPUNodes(t *testing.T) {
	c := newFakeClient(t,
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "gpu-1", Labels: map[string]string{LabelAMDGPU: "true"}}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "cpu-1"}},
	)

	nodes, err := GPUNodes(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}

	if len(nodes) != 1 || nodes[0].Name != "gpu-1" {
		t.Fatalf("GPUNodes = %v, want [gpu-1]", nodes)
	}
}
