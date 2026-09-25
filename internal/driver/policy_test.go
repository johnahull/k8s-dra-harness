package driver_test

import (
	"reflect"
	"testing"

	"github.com/johnahull/k8s-dra-harness/internal/driver"
	_ "github.com/johnahull/k8s-dra-harness/internal/driver/amd"
	_ "github.com/johnahull/k8s-dra-harness/internal/driver/cpu"
)

func TestCPUValuesAndWorkload(t *testing.T) {
	input := map[string]any{"image": map[string]any{"pullPolicy": "Always"}, "driverConfig": map[string]any{"other": "kept"}}
	cpu, err := driver.Get("cpu")
	if err != nil {
		t.Fatal(err)
	}
	values, err := cpu.Values(input, "quay.io/team/dra-driver-cpu:dev")
	if err != nil {
		t.Fatal(err)
	}
	image := values["image"].(map[string]any)
	if image["repository"] != "quay.io/team/dra-driver-cpu" || image["tag"] != "dev" || image["pullPolicy"] != "Always" {
		t.Fatalf("image values = %v", image)
	}
	config := values["driverConfig"].(map[string]any)
	if config["cpuDeviceMode"] != "individual" || config["other"] != "kept" {
		t.Fatalf("CPU driver values = %v", config)
	}
	if _, ok := input["driverConfig"].(map[string]any)["cpuDeviceMode"]; ok {
		t.Fatal("adapter changed caller values")
	}
	workload := cpu.Workload()
	if workload.Resources.Requests.Cpu().Cmp(*workload.Resources.Limits.Cpu()) != 0 || workload.Resources.Requests.Memory().Cmp(*workload.Resources.Limits.Memory()) != 0 {
		t.Fatalf("CPU workload is not Guaranteed QoS: %v", workload.Resources)
	}
}

func TestJointWorkloadPolicy(t *testing.T) {
	cpu, err := driver.Get("cpu")
	if err != nil {
		t.Fatal(err)
	}
	amd, err := driver.Get("amd")
	if err != nil {
		t.Fatal(err)
	}
	container, err := driver.JointWorkload([]driver.Adapter{cpu, amd})
	if err != nil {
		t.Fatal(err)
	}
	if container.Image != amd.Workload().Image || !reflect.DeepEqual(container.Command, []string{"/bin/sh", "-ec", "rocm-smi && env | grep '^DRA_CPUSET_' && echo PASS"}) {
		t.Fatalf("joint workload = %+v", container)
	}
	if container.Resources.Requests.Cpu().IsZero() {
		t.Fatal("joint workload lacks CPU request")
	}
	if _, err := driver.JointWorkload([]driver.Adapter{amd, amd}); err == nil {
		t.Fatal("duplicate AMD adapters should be rejected")
	}
}
