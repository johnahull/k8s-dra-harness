package config

import (
	"testing"
	"time"
)

func TestApplyPlatformDefaults(t *testing.T) {
	tests := []struct {
		name          string
		in            Config
		platform      Platform
		wantNamespace string
		wantDRANs     string
		wantMode      DriverMode
		wantDRAChart  string
	}{
		{
			name:          "openshift defaults",
			in:            Config{DRA: DRAConfig{Source: DRASourceOperator}},
			platform:      PlatformOpenShift,
			wantNamespace: NamespaceOpenShift,
			wantDRANs:     NamespaceOpenShift,
			wantMode:      DriverModeOperator,
		},
		{
			name:          "kubernetes defaults with helm DRA",
			in:            Config{DRA: DRAConfig{Source: DRASourceHelm}},
			platform:      PlatformKubernetes,
			wantNamespace: NamespaceKubernetes,
			wantDRANs:     NamespaceKubernetes,
			wantMode:      DriverModePreinstalled,
			wantDRAChart:  DefaultDRAChart,
		},
		{
			name: "explicit values win",
			in: Config{
				Namespace: "gpu",
				Driver:    DriverConfig{Mode: DriverModeOperator},
				DRA:       DRAConfig{Source: DRASourceHelm, Chart: "/src/chart", Namespace: "dra"},
			},
			platform:      PlatformKubernetes,
			wantNamespace: "gpu",
			wantDRANs:     "dra",
			wantMode:      DriverModeOperator,
			wantDRAChart:  "/src/chart",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := tt.in
			c.ApplyPlatformDefaults(tt.platform)

			if c.Platform != tt.platform {
				t.Errorf("Platform=%q, want %q", c.Platform, tt.platform)
			}
			if c.Namespace != tt.wantNamespace || c.DRA.Namespace != tt.wantDRANs {
				t.Errorf("Namespace=%q DRA.Namespace=%q, want %q %q", c.Namespace, c.DRA.Namespace, tt.wantNamespace, tt.wantDRANs)
			}
			if c.Driver.Mode != tt.wantMode {
				t.Errorf("Driver.Mode=%q, want %q", c.Driver.Mode, tt.wantMode)
			}
			if c.DRA.Chart != tt.wantDRAChart {
				t.Errorf("DRA.Chart=%q, want %q", c.DRA.Chart, tt.wantDRAChart)
			}
		})
	}
}

func TestTimeout(t *testing.T) {
	c := Config{TimeoutScale: 2.5}
	if got := c.Timeout(10 * time.Minute); got != 25*time.Minute {
		t.Errorf("Timeout=%v, want 25m", got)
	}
}
