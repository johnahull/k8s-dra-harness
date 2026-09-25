package config

import "time"

// ApplyPlatformDefaults records the detected platform and fills every
// platform-dependent field the user left unset.
func (c *Config) ApplyPlatformDefaults(p Platform) {
	c.Platform = p

	if c.Namespace == "" {
		c.Namespace = NamespaceKubernetes
		if p == PlatformOpenShift {
			c.Namespace = NamespaceOpenShift
		}
	}

	// RHCOS nodes are immutable, so OpenShift relies on KMM to build the driver;
	// Kubernetes lab nodes usually have amdgpu installed on the host.
	if c.Driver.Mode == "" {
		c.Driver.Mode = DriverModePreinstalled
		if p == PlatformOpenShift {
			c.Driver.Mode = DriverModeOperator
		}
	}

	if c.DRA.Namespace == "" {
		c.DRA.Namespace = c.Namespace
	}

	if c.DRA.Source == DRASourceHelm && c.DRA.Chart == "" {
		c.DRA.Chart = DefaultDRAChart
	}
}

// Timeout scales a default timeout by AMD_TIMEOUT_SCALE.
func (c *Config) Timeout(d time.Duration) time.Duration {
	return time.Duration(float64(d) * c.TimeoutScale)
}
