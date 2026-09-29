package runconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/johnahull/k8s-dra-harness/internal/adapters/defaults"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "run.yaml")
	if err := os.WriteFile(path, []byte("registry: quay.io/user\ndrivers:\n  - name: cpu\n    sourcePath: ./cpu\n  - name: amd\n    image: quay.io/user/amd:dev\n    chart: oci://example/amd\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Drivers[0].SourcePath != filepath.Join(dir, "cpu") || c.Workload != WorkloadPod || !c.ShouldCleanup() {
		t.Fatalf("unexpected config: %+v", c)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name   string
		config Config
		want   string
	}{
		{"missing drivers", Config{}, "at least one driver or operator"},
		{"unknown", Config{Drivers: []Driver{{Name: "sriov", Image: "x", Chart: "x"}}}, "unsupported driver"},
		{"duplicate", Config{Drivers: []Driver{{Name: "cpu", Image: "x", Chart: "x"}, {Name: "cpu", Image: "y", Chart: "y"}}}, "duplicate driver"},
		{"both sources", Config{Drivers: []Driver{{Name: "cpu", Image: "x", SourcePath: "/tmp/cpu"}}}, "exactly one"},
		{"no registry", Config{Drivers: []Driver{{Name: "cpu", SourcePath: "/tmp/cpu"}}}, "requires registry"},
		{"bundle needs package", Config{Operator: &AMDOperator{Bundle: "bundle:tag"}}, "requires package"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestOperatorOnlyConfig(t *testing.T) {
	c := Config{Operator: &AMDOperator{Chart: "./chart", Image: "quay.io/team/operator:dev"}}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	nvidia := Config{NVIDIAOperator: &NVIDIAOperator{Chart: "./chart"}}
	if err := nvidia.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := Config{Operator: &AMDOperator{Chart: "./amd"}, NVIDIAOperator: &NVIDIAOperator{Chart: "./nvidia"}}
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("Validate() = %v, want operator conflict", err)
	}
}

func TestExistingConfig(t *testing.T) {
	c := Config{Existing: true, Drivers: []Driver{{Name: "amd", Namespace: "openshift-amd-gpu"}}}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}

	bad := c
	bad.Drivers = []Driver{{Name: "cpu"}}
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "requires namespace") {
		t.Fatalf("Validate() = %v, want missing namespace error", err)
	}

	bad = c
	bad.Drivers = []Driver{{Name: "amd", Namespace: "openshift-amd-gpu", Image: "quay.io/example/amd:dev"}}
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "must not specify") {
		t.Fatalf("Validate() = %v, want artifact error", err)
	}
}

func TestPreflightConfig(t *testing.T) {
	c := Config{Preflight: true, Drivers: []Driver{{Name: "cpu", Image: "quay.io/example/cpu:dev", Chart: "oci://example/cpu"}}}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}

	bad := c
	bad.Existing = true
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("Validate() = %v, want mode conflict", err)
	}
}

func TestKubeVirtConfig(t *testing.T) {
	c := Config{
		Workload: WorkloadKubeVirt,
		KubeVirt: &KubeVirt{Namespace: "dra-kubevirt", Image: "quay.io/containerdisks/fedora:latest", Guest: &KubeVirtGuest{
			Username: "fedora", PrivateKeySecret: "ssh-key", Command: "nvidia-smi -L && echo PASS",
		}},
		Drivers: []Driver{{Name: "nvidia", Image: "quay.io/example/nvidia:dev", Chart: "oci://example/nvidia"}},
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if c.KubeVirt.Attachment != KubeVirtAttachmentGPU || c.KubeVirt.Guest.PrivateKeyKey != "id_rsa" || c.KubeVirt.Guest.ExpectedOutput != "PASS" {
		t.Fatalf("unexpected KubeVirt defaults: %+v", c.KubeVirt)
	}
	withoutNamespace := c
	withoutNamespace.KubeVirt.Namespace = ""
	if err := withoutNamespace.Validate(); err == nil || !strings.Contains(err.Error(), "namespace is required") {
		t.Fatalf("Validate() = %v, want namespace requirement", err)
	}

	bad := c
	bad.Workload = WorkloadPod
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "requires workload") {
		t.Fatalf("Validate() = %v, want workload mismatch", err)
	}
}

func TestKubeVirtRejectsCPUDriver(t *testing.T) {
	c := Config{
		Workload: WorkloadKubeVirt,
		KubeVirt: &KubeVirt{Image: "quay.io/containerdisks/fedora:latest"},
		Drivers:  []Driver{{Name: "cpu", Image: "quay.io/example/cpu:dev", Chart: "oci://example/cpu"}},
	}
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), `kubevirt workload does not support driver "cpu"`) {
		t.Fatalf("Validate() error = %v, want KubeVirt CPU rejection", err)
	}
}
