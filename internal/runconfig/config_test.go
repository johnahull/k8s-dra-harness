package runconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	if c.Drivers[0].SourcePath != filepath.Join(dir, "cpu") || !c.ShouldCleanup() {
		t.Fatalf("unexpected config: %+v", c)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name   string
		config Config
		want   string
	}{
		{"missing drivers", Config{}, "at least one driver or amdOperator"},
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
}
