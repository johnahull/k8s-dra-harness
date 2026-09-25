package config

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

const testBundleImage = "quay.io/x/bundle:1"

// clearEnv unsets every variable Load reads, restoring them when the test ends.
func clearEnv(t *testing.T) {
	t.Helper()
	extra := map[string]bool{"DRY_RUN": true, "DUMP_FAILED_TESTS": true, "REPORTS_DUMP_DIR": true, "VERBOSE_LEVEL": true}
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(key, "AMD_") || extra[key] {
			t.Setenv(key, "")
			if err := os.Unsetenv(key); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestLoadDefaults(t *testing.T) {
	clearEnv(t)

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if !c.Cleanup || !c.InstallDeps {
		t.Errorf("Cleanup=%v InstallDeps=%v, want both true", c.Cleanup, c.InstallDeps)
	}
	if c.TimeoutScale != 1.0 {
		t.Errorf("TimeoutScale=%v, want 1.0", c.TimeoutScale)
	}
	if c.ReportsDir != "/tmp/reports" {
		t.Errorf("ReportsDir=%q", c.ReportsDir)
	}
	if c.Operator.Source != OperatorSourceRelease || c.Operator.Channel != "alpha" ||
		c.Operator.Catalog != "certified-operators" || c.Operator.Chart != DefaultOperatorChart {
		t.Errorf("Operator defaults wrong: %+v", c.Operator)
	}
	if c.DRA.Source != DRASourceOperator {
		t.Errorf("DRA.Source=%q, want operator", c.DRA.Source)
	}
	// Platform-dependent values stay empty until ApplyPlatformDefaults.
	if c.Platform != "" || c.Namespace != "" || c.Driver.Mode != "" || c.DRA.Chart != "" {
		t.Errorf("platform-dependent fields should be empty: %+v", c)
	}
	if c.Workload.Image != DefaultWorkloadImage || c.Workload.Duration.String() != "1m0s" {
		t.Errorf("Workload defaults wrong: %+v", c.Workload)
	}
}

func TestLoadFromEnv(t *testing.T) {
	clearEnv(t)
	t.Setenv("AMD_PLATFORM", "kubernetes")
	t.Setenv("AMD_CLEANUP", "false")
	t.Setenv("AMD_OPERATOR_SOURCE", "custom")
	t.Setenv("AMD_OPERATOR_BUNDLE", testBundleImage)
	t.Setenv("AMD_DRIVER_MODE", "preinstalled")
	t.Setenv("AMD_DRA_SOURCE", "helm")
	t.Setenv("AMD_DRA_ARGS", "v=4, feature-gates=A=true")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if c.Platform != PlatformKubernetes || c.Cleanup {
		t.Errorf("Platform=%q Cleanup=%v", c.Platform, c.Cleanup)
	}
	if c.Operator.Source != OperatorSourceCustom || c.Operator.Bundle != testBundleImage {
		t.Errorf("Operator=%+v", c.Operator)
	}
	if c.Driver.Mode != DriverModePreinstalled || c.DRA.Source != DRASourceHelm {
		t.Errorf("Driver=%+v DRA=%+v", c.Driver, c.DRA)
	}
	want := KeyValues{"v": "4", "feature-gates": "A=true"}
	if !reflect.DeepEqual(c.DRA.Args, want) {
		t.Errorf("DRA.Args=%v, want %v", c.DRA.Args, want)
	}
}

func TestKeyValuesDecode(t *testing.T) {
	var kv KeyValues
	if err := kv.Decode(""); err != nil || len(kv) != 0 {
		t.Errorf("empty: kv=%v err=%v", kv, err)
	}
	if err := kv.Decode("novalue"); err == nil {
		t.Error("expected error for pair without '='")
	}
	if err := kv.Decode("=v"); err == nil {
		t.Error("expected error for empty key")
	}
}
