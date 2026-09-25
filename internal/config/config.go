// Package config loads amd-ci settings from environment variables.
package config

import (
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"
)

// Platform is the kind of cluster under test.
type Platform string

// Supported platforms.
const (
	PlatformOpenShift  Platform = "openshift"
	PlatformKubernetes Platform = "kubernetes"
)

// DriverMode selects who provides the amdgpu kernel module.
type DriverMode string

// Supported driver modes.
const (
	DriverModeOperator     DriverMode = "operator"
	DriverModePreinstalled DriverMode = "preinstalled"
)

// OperatorSource selects where the AMD GPU Operator is installed from.
type OperatorSource string

// Supported operator sources.
const (
	OperatorSourceRelease OperatorSource = "release"
	OperatorSourceCustom  OperatorSource = "custom"
)

// DRASource selects how the DRA driver is installed.
type DRASource string

// Supported DRA sources.
const (
	DRASourceOperator DRASource = "operator"
	DRASourceHelm     DRASource = "helm"
)

// Defaults that do not depend on the platform.
const (
	NamespaceOpenShift   = "openshift-amd-gpu"
	NamespaceKubernetes  = "kube-amd-gpu"
	DefaultOperatorChart = "rocm/gpu-operator-charts"
	DefaultDRAChart      = "rocm-k8s-gpu-dra-driver/k8s-gpu-dra-driver"
	DefaultWorkloadImage = "docker.io/rocm/dev-ubuntu-22.04:6.4"
)

// Config holds every amd-ci setting. Fields documented as platform-dependent
// are empty after Load and filled by ApplyPlatformDefaults.
type Config struct {
	Platform        Platform `envconfig:"AMD_PLATFORM"`
	Cleanup         bool     `envconfig:"AMD_CLEANUP" default:"true"`
	Namespace       string   `envconfig:"AMD_NAMESPACE"` // platform-dependent
	TimeoutScale    float64  `envconfig:"AMD_TIMEOUT_SCALE" default:"1.0"`
	InstallDeps     bool     `envconfig:"AMD_INSTALL_DEPS" default:"true"`
	DumpFailedTests bool     `envconfig:"DUMP_FAILED_TESTS" default:"false"`
	ReportsDir      string   `envconfig:"REPORTS_DUMP_DIR" default:"/tmp/reports"`
	DryRun          bool     `envconfig:"DRY_RUN" default:"false"`
	VerboseLevel    string   `envconfig:"VERBOSE_LEVEL" default:"0"`

	Operator OperatorConfig
	Driver   DriverConfig
	DRA      DRAConfig
	Workload WorkloadConfig
}

// OperatorConfig selects the AMD GPU Operator build to install.
type OperatorConfig struct {
	Source  OperatorSource `envconfig:"AMD_OPERATOR_SOURCE" default:"release"`
	Version string         `envconfig:"AMD_OPERATOR_VERSION"`
	Channel string         `envconfig:"AMD_OPERATOR_CHANNEL" default:"alpha"`
	Catalog string         `envconfig:"AMD_OPERATOR_CATALOG" default:"certified-operators"`
	Bundle  string         `envconfig:"AMD_OPERATOR_BUNDLE"`
	Chart   string         `envconfig:"AMD_OPERATOR_CHART" default:"rocm/gpu-operator-charts"`
	Image   string         `envconfig:"AMD_OPERATOR_IMAGE"`
}

// DriverConfig selects the amdgpu kernel driver.
type DriverConfig struct {
	Mode    DriverMode `envconfig:"AMD_DRIVER_MODE"` // platform-dependent
	Version string     `envconfig:"AMD_DRIVER_VERSION"`
	Image   string     `envconfig:"AMD_DRIVER_IMAGE"`
}

// DRAConfig selects the DRA driver build and install method.
type DRAConfig struct {
	Source    DRASource `envconfig:"AMD_DRA_SOURCE" default:"operator"`
	Image     string    `envconfig:"AMD_DRA_IMAGE"`
	Chart     string    `envconfig:"AMD_DRA_CHART"`     // defaulted only when Source is helm
	Namespace string    `envconfig:"AMD_DRA_NAMESPACE"` // defaults to Namespace
	Args      KeyValues `envconfig:"AMD_DRA_ARGS"`
}

// WorkloadConfig selects the ROCm test workload.
type WorkloadConfig struct {
	Image    string        `envconfig:"AMD_WORKLOAD_IMAGE" default:"docker.io/rocm/dev-ubuntu-22.04:6.4"`
	Duration time.Duration `envconfig:"AMD_WORKLOAD_DURATION" default:"60s"`
}

// Load reads Config from the environment. Nested structs are looked up by
// their explicit envconfig tag (envconfig's "Alt" key), so no prefixes apply.
func Load() (*Config, error) {
	var c Config

	if err := envconfig.Process("", &c); err != nil {
		return nil, fmt.Errorf("reading environment: %w", err)
	}

	return &c, nil
}
