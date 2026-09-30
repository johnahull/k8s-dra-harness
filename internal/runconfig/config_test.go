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
		{"unknown", Config{Drivers: []Driver{{Name: "not-a-driver", Image: "x", Chart: "x"}}}, "unsupported driver"},
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

func TestSriovPolicyConfig(t *testing.T) {
	c := Config{Registry: "quay.io/team", Drivers: []Driver{{
		Name: "sriov", SourcePath: "/tmp/sriov", SriovPolicy: &SriovPolicy{
			Name: "all-devices", Spec: map[string]any{"configs": []any{map[string]any{}}},
		},
	}}}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}

	bad := c
	bad.Drivers = []Driver{{Name: "cpu", SourcePath: "/tmp/cpu", SriovPolicy: c.Drivers[0].SriovPolicy}}
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "cannot configure sriovPolicy") {
		t.Fatalf("Validate() = %v, want adapter restriction", err)
	}

	bad = c
	bad.Drivers = append([]Driver(nil), c.Drivers...)
	bad.Drivers[0].SriovPolicy = &SriovPolicy{}
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "requires spec") {
		t.Fatalf("Validate() = %v, want policy spec requirement", err)
	}

	bad = c
	bad.Drivers = append([]Driver(nil), c.Drivers...)
	bad.Existing = true
	bad.Drivers[0].Namespace = "dra-sriov"
	bad.Drivers[0].SourcePath = ""
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "cannot create sriovPolicy") {
		t.Fatalf("Validate() = %v, want existing-mode policy restriction", err)
	}

	bad = c
	bad.Drivers = append([]Driver(nil), c.Drivers...)
	bad.Drivers[0].Namespace = "driver-ns"
	bad.Drivers[0].SriovPolicy = &SriovPolicy{Name: "all-devices", Namespace: "policy-ns", Spec: map[string]any{"configs": []any{map[string]any{}}}}
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "must match driver namespace") {
		t.Fatalf("Validate() = %v, want namespace consistency error", err)
	}
}

func TestLoadSriovPolicy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sriov.yaml")
	data := `registry: quay.io/team
drivers:
  - name: sriov
    sourcePath: /tmp/dra-driver-sriov
    claimConfig:
      requests: [device]
      driver: sriovnetwork.k8snetworkplumbingwg.io
      parameters:
        apiVersion: sriovnetwork.k8snetworkplumbingwg.io/v1alpha1
        kind: VfConfig
    sriovPolicy:
      namespace: sriov-driver
      spec:
        configs:
          - {}
`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	policy := c.Drivers[0].SriovPolicy
	if policy == nil || policy.Namespace != "sriov-driver" || len(policy.Spec) != 1 || c.Drivers[0].ClaimConfig == nil {
		t.Fatalf("unexpected SR-IOV policy: %+v", policy)
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

func TestKubeVirtVFIOClaimConfig(t *testing.T) {
	c := Config{
		Existing: true,
		Workload: WorkloadKubeVirt,
		KubeVirt: &KubeVirt{
			AllowExisting: true,
			Image:         "quay.io/containerdisks/fedora:latest",
			Selector:      `device.attributes["gpu.amd.com"].type == "vfio"`,
			ClaimConfig: &KubeVirtClaim{
				Driver: "gpu.amd.com",
				Parameters: map[string]any{
					"apiVersion": "gpu.resource.amd.com/v1alpha1",
					"kind":       "VfioDeviceConfig",
				},
			},
		},
		Drivers: []Driver{{Name: "amd", Namespace: "dra-amd"}},
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if c.KubeVirt.Attachment != KubeVirtAttachmentGPU {
		t.Fatalf("attachment = %q, want %q", c.KubeVirt.Attachment, KubeVirtAttachmentGPU)
	}
}

func TestKubeVirtRejectsExistingOptInWithoutExistingMode(t *testing.T) {
	c := Config{
		Workload: WorkloadKubeVirt,
		KubeVirt: &KubeVirt{AllowExisting: true, Image: "quay.io/containerdisks/fedora:latest"},
		Drivers:  []Driver{{Name: "amd", Image: "quay.io/example/amd:dev", Chart: "oci://example/amd"}},
	}
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "requires existing") {
		t.Fatalf("Validate() = %v, want existing-mode error", err)
	}
}

func TestKubeVirtRejectsNegativeReadyHold(t *testing.T) {
	c := Config{
		Workload: WorkloadKubeVirt,
		KubeVirt: &KubeVirt{Image: "quay.io/containerdisks/fedora:latest", HoldAfterReadySeconds: -1},
		Drivers:  []Driver{{Name: "amd", Image: "quay.io/example/amd:dev", Chart: "oci://example/amd"}},
	}
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "holdAfterReadySeconds") {
		t.Fatalf("Validate() = %v, want negative ready-hold error", err)
	}
}

func TestTestPlanLoadResolvesPaths(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "run.yaml")
	data := `registry: quay.io/user
drivers:
  - name: amd
    sourcePath: ./amd
testPlan:
  profile: amd-pr-91-topology
  scenarios: [resource-slices, counters]
  verification:
    scriptsDir: ./scripts
    evidenceDir: ./evidence
`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := c.TestPlan.Verification.ScriptsDir, filepath.Join(dir, "scripts"); got != want {
		t.Fatalf("scriptsDir=%q, want %q", got, want)
	}
	if got, want := c.TestPlan.Verification.EvidenceDir, filepath.Join(dir, "evidence"); got != want {
		t.Fatalf("evidenceDir=%q, want %q", got, want)
	}
}

func TestTestPlanValidation(t *testing.T) {
	base := Config{Drivers: []Driver{{Name: "amd", Image: "quay.io/user/amd:dev", Chart: "oci://example/amd"}}}
	tests := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{"missing profile", func(c *Config) { c.TestPlan = &TestPlan{Scenarios: []string{"resource-slices"}} }, "requires profile"},
		{"unknown scenario", func(c *Config) { c.TestPlan = &TestPlan{Profile: "x", Scenarios: []string{"unknown"}} }, "unsupported testPlan scenario"},
		{"restart permission", func(c *Config) {
			c.TestPlan = &TestPlan{Profile: "x", Scenarios: []string{"restart"}, Lifecycle: TestPlanLifecycle{AllowRestart: true}}
		}, "allowRestart requires allowWorkloads"},
		{"workload permission", func(c *Config) {
			c.TestPlan = &TestPlan{Profile: "x", Scenarios: []string{"release"}}
		}, "requires lifecycle.allowWorkloads"},
		{"AMD scenario driver", func(c *Config) {
			c.Drivers = []Driver{{Name: "cpu", Image: "quay.io/user/cpu:dev", Chart: "oci://example/cpu"}}
			c.TestPlan = &TestPlan{Profile: "x", Scenarios: []string{"capacity"}, Lifecycle: TestPlanLifecycle{AllowWorkloads: true}}
		}, "requires the amd driver"},
		{"scripts evidence", func(c *Config) {
			c.TestPlan = &TestPlan{Profile: "x", Scenarios: []string{"resource-slices"}, Verification: TestPlanVerification{ScriptsDir: "/tmp/scripts"}}
		}, "scriptsDir requires evidenceDir"},
		{"topology request", func(c *Config) {
			c.TestPlan = &TestPlan{Profile: "x", Scenarios: []string{"topology"}, Topology: []TopologyCase{{Name: "bad", Requests: []TopologyRequest{{Name: "gpu"}}}}}
		}, "require name and deviceClass"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := base
			tt.mutate(&c)
			if err := c.Validate(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate()=%v, want %q", err, tt.want)
			}
		})
	}
}

func TestTestPlanRejectsKubeVirt(t *testing.T) {
	c := Config{
		Workload: WorkloadKubeVirt,
		KubeVirt: &KubeVirt{Image: "quay.io/containerdisks/fedora:latest"},
		Drivers:  []Driver{{Name: "amd", Image: "quay.io/user/amd:dev", Chart: "oci://example/amd"}},
		TestPlan: &TestPlan{Profile: "x", Scenarios: []string{"resource-slices"}},
	}
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "testPlan cannot be combined") {
		t.Fatalf("Validate()=%v, want KubeVirt/testPlan conflict", err)
	}
}
