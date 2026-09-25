// Package runconfig loads the cluster and driver selection for an end-to-end run.
package runconfig

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/johnahull/k8s-dra-harness/internal/driver"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/util/validation"
)

// Config describes one run against an existing cluster.
type Config struct {
	Namespace      string          `yaml:"namespace"`
	Registry       string          `yaml:"registry"`
	Cleanup        *bool           `yaml:"cleanup"`
	Existing       bool            `yaml:"existing"`
	Preflight      bool            `yaml:"preflight"`
	Drivers        []Driver        `yaml:"drivers"`
	Operator       *AMDOperator    `yaml:"amdOperator"`
	NVIDIAOperator *NVIDIAOperator `yaml:"nvidiaOperator"`
}

// Driver selects an adapter and either a local checkout or a published image.
type Driver struct {
	Name          string         `yaml:"name"`
	SourcePath    string         `yaml:"sourcePath"`
	Image         string         `yaml:"image"`
	Chart         string         `yaml:"chart"`
	Namespace     string         `yaml:"namespace"`
	Values        map[string]any `yaml:"values"`
	UpstreamTests []string       `yaml:"upstreamTests"`
}

// AMDOperator is an optional prerequisite for an operator-managed AMD driver.
type AMDOperator struct {
	Chart        string         `yaml:"chart"`
	Image        string         `yaml:"image"`
	Bundle       string         `yaml:"bundle"`
	Package      string         `yaml:"package"`
	Namespace    string         `yaml:"namespace"`
	Values       map[string]any `yaml:"values"`
	DeviceConfig map[string]any `yaml:"deviceConfig"`
}

// NVIDIAOperator is an optional prerequisite for an NVIDIA GPU driver. The
// harness installs the GPU Operator chart in classic ClusterPolicy mode; the
// standalone NVIDIA DRA adapter owns DRA resources separately.
type NVIDIAOperator struct {
	Chart     string         `yaml:"chart"`
	Image     string         `yaml:"image"`
	Namespace string         `yaml:"namespace"`
	Values    map[string]any `yaml:"values"`
}

// Load reads a strict YAML configuration, resolving checkout and chart paths
// relative to the configuration file rather than the caller's working directory.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading run config: %w", err)
	}
	var c Config
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("decoding run config: %w", err)
	}
	base := filepath.Dir(path)
	for i := range c.Drivers {
		d := &c.Drivers[i]
		if d.SourcePath != "" && !filepath.IsAbs(d.SourcePath) {
			d.SourcePath = filepath.Join(base, d.SourcePath)
		}
		if d.Chart != "" && strings.HasPrefix(d.Chart, ".") && !filepath.IsAbs(d.Chart) {
			d.Chart = filepath.Join(base, d.Chart)
		}
	}
	if c.Operator != nil && strings.HasPrefix(c.Operator.Chart, ".") && !filepath.IsAbs(c.Operator.Chart) {
		c.Operator.Chart = filepath.Join(base, c.Operator.Chart)
	}
	if c.NVIDIAOperator != nil && strings.HasPrefix(c.NVIDIAOperator.Chart, ".") && !filepath.IsAbs(c.NVIDIAOperator.Chart) {
		c.NVIDIAOperator.Chart = filepath.Join(base, c.NVIDIAOperator.Chart)
	}
	if c.Namespace == "" {
		c.Namespace = "dra-harness"
	}
	return &c, c.Validate()
}

// Validate rejects ambiguous artifact sources and unsupported adapters before
// any cluster resource is changed.
func (c *Config) Validate() error {
	var problems []error
	namespace := c.Namespace
	if namespace == "" {
		namespace = "dra-harness"
	}
	if len(namespace) > 40 || len(validation.IsDNS1123Label(namespace)) > 0 {
		problems = append(problems, errors.New("namespace must be a DNS label of at most 40 characters"))
	}
	if len(c.Drivers) == 0 && c.Operator == nil && c.NVIDIAOperator == nil {
		problems = append(problems, errors.New("at least one driver or operator is required"))
	}
	seen := map[string]bool{}
	for _, d := range c.Drivers {
		if _, err := driver.Get(d.Name); err != nil {
			problems = append(problems, err)
		}
		if seen[d.Name] {
			problems = append(problems, fmt.Errorf("duplicate driver %q", d.Name))
		}
		seen[d.Name] = true
		if c.Existing {
			if d.Namespace == "" {
				problems = append(problems, fmt.Errorf("existing driver %q requires namespace", d.Name))
			}
			if d.SourcePath != "" || d.Image != "" || d.Chart != "" {
				problems = append(problems, fmt.Errorf("existing driver %q must not specify sourcePath, image, or chart", d.Name))
			}
		} else {
			if (d.SourcePath == "") == (d.Image == "") {
				problems = append(problems, fmt.Errorf("driver %q requires exactly one of sourcePath or image", d.Name))
			}
			if d.SourcePath == "" && d.Chart == "" {
				problems = append(problems, fmt.Errorf("driver %q requires chart when using image", d.Name))
			}
			if d.SourcePath != "" && c.Registry == "" {
				problems = append(problems, fmt.Errorf("driver %q source build requires registry", d.Name))
			}
		}
		if d.Namespace != "" && len(validation.IsDNS1123Label(d.Namespace)) > 0 {
			problems = append(problems, fmt.Errorf("driver %q namespace must be a DNS label", d.Name))
		}
		if len(d.UpstreamTests) > 0 && (c.Existing || d.SourcePath == "") {
			problems = append(problems, fmt.Errorf("driver %q upstreamTests requires sourcePath", d.Name))
		}
	}
	if c.Operator != nil && c.NVIDIAOperator != nil {
		problems = append(problems, errors.New("amdOperator and nvidiaOperator are mutually exclusive"))
	}
	if c.Existing && (c.Operator != nil || c.NVIDIAOperator != nil) {
		problems = append(problems, errors.New("existing mode cannot select an operator"))
	}
	if c.Preflight && c.Existing {
		problems = append(problems, errors.New("preflight and existing modes are mutually exclusive"))
	}
	if c.Preflight && (c.Operator != nil || c.NVIDIAOperator != nil) {
		problems = append(problems, errors.New("preflight mode cannot select an operator"))
	}
	if c.Operator != nil {
		if c.Operator.Chart == "" && c.Operator.Bundle == "" {
			problems = append(problems, errors.New("amdOperator requires chart or bundle"))
		}
		if c.Operator.Chart != "" && c.Operator.Bundle != "" {
			problems = append(problems, errors.New("amdOperator chart and bundle are mutually exclusive"))
		}
		if c.Operator.Bundle != "" && c.Operator.Package == "" {
			problems = append(problems, errors.New("amdOperator bundle requires package for safe cleanup"))
		}
		if c.Operator.Bundle != "" && len(c.Operator.DeviceConfig) == 0 {
			problems = append(problems, errors.New("amdOperator bundle requires deviceConfig spec"))
		}
	}
	if c.NVIDIAOperator != nil && c.NVIDIAOperator.Chart == "" {
		problems = append(problems, errors.New("nvidiaOperator requires chart"))
	}
	if c.NVIDIAOperator != nil && c.NVIDIAOperator.Namespace != "" && len(validation.IsDNS1123Label(c.NVIDIAOperator.Namespace)) > 0 {
		problems = append(problems, errors.New("nvidiaOperator namespace must be a DNS label"))
	}
	return errors.Join(problems...)
}

// ShouldCleanup defaults to true.
func (c *Config) ShouldCleanup() bool { return c.Cleanup == nil || *c.Cleanup }
