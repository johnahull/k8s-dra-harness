package config

import (
	"errors"
	"fmt"
)

// Validate rejects unknown enum values and invalid combinations. It must run
// after ApplyPlatformDefaults. All problems are reported together.
func (c *Config) Validate() error {
	var errs []error

	switch c.Platform {
	case PlatformOpenShift, PlatformKubernetes:
	default:
		errs = append(errs, fmt.Errorf("AMD_PLATFORM must be %q or %q, got %q", PlatformOpenShift, PlatformKubernetes, c.Platform))
	}

	switch c.Operator.Source {
	case OperatorSourceRelease, OperatorSourceCustom:
	default:
		errs = append(errs, fmt.Errorf("AMD_OPERATOR_SOURCE must be %q or %q, got %q", OperatorSourceRelease, OperatorSourceCustom, c.Operator.Source))
	}

	switch c.Driver.Mode {
	case DriverModeOperator, DriverModePreinstalled:
	default:
		errs = append(errs, fmt.Errorf("AMD_DRIVER_MODE must be %q or %q, got %q", DriverModeOperator, DriverModePreinstalled, c.Driver.Mode))
	}

	switch c.DRA.Source {
	case DRASourceOperator, DRASourceHelm:
	default:
		errs = append(errs, fmt.Errorf("AMD_DRA_SOURCE must be %q or %q, got %q", DRASourceOperator, DRASourceHelm, c.DRA.Source))
	}

	if c.TimeoutScale <= 0 {
		errs = append(errs, fmt.Errorf("AMD_TIMEOUT_SCALE must be > 0, got %v", c.TimeoutScale))
	}

	if c.Operator.Source == OperatorSourceCustom {
		switch c.Platform {
		case PlatformOpenShift:
			if c.Operator.Bundle == "" {
				errs = append(errs, errors.New("AMD_OPERATOR_SOURCE=custom on OpenShift requires AMD_OPERATOR_BUNDLE"))
			}
		case PlatformKubernetes:
			if c.Operator.Chart == DefaultOperatorChart && c.Operator.Image == "" {
				errs = append(errs, errors.New("AMD_OPERATOR_SOURCE=custom on Kubernetes requires a non-default AMD_OPERATOR_CHART or AMD_OPERATOR_IMAGE"))
			}
		}
	}

	if c.Platform == PlatformKubernetes && c.Operator.Bundle != "" {
		errs = append(errs, errors.New("AMD_OPERATOR_BUNDLE is only supported on OpenShift; use AMD_OPERATOR_CHART on Kubernetes"))
	}

	if c.DRA.Source == DRASourceOperator && c.DRA.Chart != "" {
		errs = append(errs, errors.New("AMD_DRA_CHART requires AMD_DRA_SOURCE=helm"))
	}

	return errors.Join(errs...)
}
