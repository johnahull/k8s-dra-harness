package config

import (
	"strings"
	"testing"
)

// validConfig returns a config that passes Validate on the given platform.
func validConfig(p Platform) Config {
	c := Config{
		TimeoutScale: 1,
		Operator:     OperatorConfig{Source: OperatorSourceRelease, Chart: DefaultOperatorChart},
		DRA:          DRAConfig{Source: DRASourceOperator},
	}
	c.ApplyPlatformDefaults(p)

	return c
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(c *Config)
		p       Platform
		wantErr string // empty means valid
	}{
		{name: "openshift defaults valid", p: PlatformOpenShift, mutate: func(*Config) {}},
		{name: "kubernetes defaults valid", p: PlatformKubernetes, mutate: func(*Config) {}},
		{
			name: "unknown platform", p: PlatformKubernetes,
			mutate:  func(c *Config) { c.Platform = "rancher" },
			wantErr: "AMD_PLATFORM",
		},
		{
			name: "unknown operator source", p: PlatformKubernetes,
			mutate:  func(c *Config) { c.Operator.Source = "nightly" },
			wantErr: "AMD_OPERATOR_SOURCE",
		},
		{
			name: "unknown driver mode", p: PlatformKubernetes,
			mutate:  func(c *Config) { c.Driver.Mode = "dkms" },
			wantErr: "AMD_DRIVER_MODE",
		},
		{
			name: "unknown DRA source", p: PlatformKubernetes,
			mutate:  func(c *Config) { c.DRA.Source = "kustomize" },
			wantErr: "AMD_DRA_SOURCE",
		},
		{
			name: "non-positive timeout scale", p: PlatformKubernetes,
			mutate:  func(c *Config) { c.TimeoutScale = 0 },
			wantErr: "AMD_TIMEOUT_SCALE",
		},
		{
			name: "custom operator on openshift needs bundle", p: PlatformOpenShift,
			mutate:  func(c *Config) { c.Operator.Source = OperatorSourceCustom },
			wantErr: "AMD_OPERATOR_BUNDLE",
		},
		{
			name: "custom operator on openshift with bundle", p: PlatformOpenShift,
			mutate: func(c *Config) {
				c.Operator.Source = OperatorSourceCustom
				c.Operator.Bundle = testBundleImage
			},
		},
		{
			name: "custom operator on kubernetes needs chart or image", p: PlatformKubernetes,
			mutate:  func(c *Config) { c.Operator.Source = OperatorSourceCustom },
			wantErr: "AMD_OPERATOR_CHART",
		},
		{
			name: "custom operator on kubernetes with local chart", p: PlatformKubernetes,
			mutate: func(c *Config) {
				c.Operator.Source = OperatorSourceCustom
				c.Operator.Chart = "/src/gpu-operator/helm-charts-k8s"
			},
		},
		{
			name: "custom operator on kubernetes with image only", p: PlatformKubernetes,
			mutate: func(c *Config) {
				c.Operator.Source = OperatorSourceCustom
				c.Operator.Image = "quay.io/x/operator:dev"
			},
		},
		{
			name: "bundle on kubernetes", p: PlatformKubernetes,
			mutate:  func(c *Config) { c.Operator.Bundle = testBundleImage },
			wantErr: "only supported on OpenShift",
		},
		{
			name: "DRA chart with operator source", p: PlatformKubernetes,
			mutate:  func(c *Config) { c.DRA.Chart = "/src/chart" },
			wantErr: "AMD_DRA_CHART requires AMD_DRA_SOURCE=helm",
		},
		{
			name: "DRA chart with helm source", p: PlatformKubernetes,
			mutate: func(c *Config) {
				c.DRA.Source = DRASourceHelm
				c.DRA.Chart = "/src/chart"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validConfig(tt.p)
			tt.mutate(&c)
			err := c.Validate()

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				return
			}

			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateReportsAllErrors(t *testing.T) {
	c := validConfig(PlatformKubernetes)
	c.TimeoutScale = -1
	c.DRA.Source = "bogus"

	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "AMD_TIMEOUT_SCALE") || !strings.Contains(err.Error(), "AMD_DRA_SOURCE") {
		t.Fatalf("want both errors reported, got %v", err)
	}
}
