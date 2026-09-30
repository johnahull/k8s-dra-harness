package harness

import "testing"

func TestGroupedCPUValues(t *testing.T) {
	input := map[string]any{"driverConfig": map[string]any{"other": "kept", "cpuDeviceMode": "individual"}}
	got := groupedCPUValues(input)
	config := got["driverConfig"].(map[string]any)
	if config["cpuDeviceMode"] != "grouped" || config["other"] != "kept" {
		t.Fatalf("grouped CPU values = %v", config)
	}
	if input["driverConfig"].(map[string]any)["cpuDeviceMode"] != "individual" {
		t.Fatal("groupedCPUValues changed caller values")
	}
}
