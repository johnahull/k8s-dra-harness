package sriov

import (
	"strings"
	"testing"
)

func TestAdapterMetadata(t *testing.T) {
	a := adapter{}
	if a.Name() != "sriov" || a.DriverName() != "sriovnetwork.k8snetworkplumbingwg.io" || a.DeviceClass() != a.DriverName() {
		t.Fatalf("unexpected adapter metadata: name=%q driver=%q class=%q", a.Name(), a.DriverName(), a.DeviceClass())
	}
	if a.ChartDir() != "deployments/helm/dra-driver-sriov" {
		t.Fatalf("ChartDir() = %q", a.ChartDir())
	}
	if got := a.Image("quay.io/team", "dev"); got != "quay.io/team/dra-driver-sriov:dev" {
		t.Fatalf("Image() = %q", got)
	}
}

func TestValuesAndWorkload(t *testing.T) {
	a := adapter{}
	values, err := a.Values(nil, "quay.io/team/dra-driver-sriov:dev")
	if err != nil {
		t.Fatal(err)
	}
	image := values["image"].(map[string]any)
	if image["repository"] != "quay.io/team/dra-driver-sriov" || image["tag"] != "dev" {
		t.Fatalf("image values = %#v", image)
	}
	plugin := values["kubeletPlugin"].(map[string]any)
	if plugin["configurationMode"] != "MULTUS" {
		t.Fatalf("configurationMode = %v", plugin["configurationMode"])
	}

	configured, err := a.Values(map[string]any{"kubeletPlugin": map[string]any{"configurationMode": "STANDALONE"}}, "quay.io/team/dra-driver-sriov:dev")
	if err != nil {
		t.Fatal(err)
	}
	if got := configured["kubeletPlugin"].(map[string]any)["configurationMode"]; got != "STANDALONE" {
		t.Fatalf("explicit configurationMode = %v", got)
	}
	if command := strings.Join(a.Workload().Command, " "); !strings.Contains(command, "SRIOVNETWORK_VF_DEVICE_") || !strings.Contains(command, "PASS") {
		t.Fatalf("workload command = %q", command)
	}
}
