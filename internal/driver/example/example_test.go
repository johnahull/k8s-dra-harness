package example

import (
	"strings"
	"testing"
)

const exampleDriverName = "gpu.example.com"

func TestAdapterMetadata(t *testing.T) {
	a := adapter{}
	if a.Name() != "example" || a.DriverName() != exampleDriverName || a.DeviceClass() != exampleDriverName {
		t.Fatalf("unexpected adapter metadata: name=%q driver=%q class=%q", a.Name(), a.DriverName(), a.DeviceClass())
	}
	if got := a.Image("quay.io/team", "dev"); got != "quay.io/team/dra-example-driver/dra-example-driver:dev" {
		t.Fatalf("Image() = %q", got)
	}
}

func TestValuesAndWorkload(t *testing.T) {
	a := adapter{}
	values, err := a.Values(nil, "quay.io/team/example:dev")
	if err != nil {
		t.Fatal(err)
	}
	if values["deviceProfile"] != "gpu" {
		t.Fatalf("deviceProfile = %v", values["deviceProfile"])
	}
	image := values["image"].(map[string]any)
	if image["repository"] != "quay.io/team/example" || image["tag"] != "dev" {
		t.Fatalf("image values = %#v", image)
	}
	if command := strings.Join(a.Workload().Command, " "); !strings.Contains(command, "GPU_DEVICE_") || !strings.Contains(command, "PASS") {
		t.Fatalf("workload command = %q", command)
	}
}
