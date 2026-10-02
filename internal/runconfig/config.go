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
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation"
)

// Config describes one run against an existing cluster.
type Config struct {
	Namespace      string          `yaml:"namespace"`
	Registry       string          `yaml:"registry"`
	Cleanup        *bool           `yaml:"cleanup"`
	Existing       bool            `yaml:"existing"`
	Preflight      bool            `yaml:"preflight"`
	Workload       string          `yaml:"workload"`
	TestPlan       *TestPlan       `yaml:"testPlan"`
	KubeVirt       *KubeVirt       `yaml:"kubevirt"`
	Drivers        []Driver        `yaml:"drivers"`
	Operator       *AMDOperator    `yaml:"amdOperator"`
	NVIDIAOperator *NVIDIAOperator `yaml:"nvidiaOperator"`
}

const (
	WorkloadPod                  = "pod"
	WorkloadKubeVirt             = "kubevirt"
	KubeVirtAttachmentGPU        = "gpu"
	KubeVirtAttachmentHostDevice = "hostDevice"
	KubeVirtAttachmentCPU        = "cpu"
	KubeVirtAttachmentNetwork    = "network"
)

// KubeVirt selects the direct-VMI workload backend. The referenced image and
// secrets must already be available to the target cluster; the harness never
// creates cluster-wide KubeVirt configuration or guest credentials.
type KubeVirt struct {
	Namespace  string `yaml:"namespace"`
	Image      string `yaml:"image"`
	Attachment string `yaml:"attachment"`
	DeviceName string `yaml:"deviceName"`
	// CPUModel overrides KubeVirt's default CPU model for VM compatibility
	// tests. host-passthrough is required when testing large PCI apertures.
	CPUModel string `yaml:"cpuModel"`
	// NetworkBinding selects the binding for the default pod network. An
	// explicit masquerade binding avoids requiring a pod-network gateway when
	// the test only needs the VM to boot.
	NetworkBinding string `yaml:"networkBinding"`
	// DeviceCount controls how many DRA requests and matching KubeVirt
	// devices are placed in the workload. Zero preserves the single-device
	// default for backward compatibility.
	DeviceCount   int            `yaml:"deviceCount"`
	Selector      string         `yaml:"selector"`
	ClaimConfig   *KubeVirtClaim `yaml:"claimConfig"`
	AllowExisting bool           `yaml:"allowExisting"`
	// HoldAfterReadySeconds keeps a live VMI and claim present after readiness.
	// It is intended for lifecycle tests that mutate the driver while the VMI
	// is using an allocated device.
	HoldAfterReadySeconds int            `yaml:"holdAfterReadySeconds"`
	CloudInitSecret       string         `yaml:"cloudInitSecret"`
	Guest                 *KubeVirtGuest `yaml:"guest"`
}

// ClaimConfig configures opaque driver parameters for a ResourceClaim.
type ClaimConfig struct {
	Requests   []string       `yaml:"requests"`
	Driver     string         `yaml:"driver"`
	Parameters map[string]any `yaml:"parameters"`
}

// KubeVirtClaim configures the ResourceClaim consumed by a KubeVirt VMI.
// Capacity is used by capacity-based drivers such as the grouped CPU DRA
// driver; Driver and Parameters configure opaque device requests such as
// SR-IOV VF setup.
type KubeVirtClaim struct {
	Requests   []string       `yaml:"requests"`
	Driver     string         `yaml:"driver"`
	Parameters map[string]any `yaml:"parameters"`
	Capacity   map[string]any `yaml:"capacity"`
}

// KubeVirtGuest describes optional in-guest verification through virtctl ssh.
// The private key is read from a Kubernetes Secret and is never written to the
// run configuration or included in workload objects.
type KubeVirtGuest struct {
	Virtctl          string `yaml:"virtctl"`
	Username         string `yaml:"username"`
	PrivateKeySecret string `yaml:"privateKeySecret"`
	PrivateKeyKey    string `yaml:"privateKeyKey"`
	Command          string `yaml:"command"`
	ExpectedOutput   string `yaml:"expectedOutput"`
}

// Driver selects an adapter and either a local checkout or a published image.
type Driver struct {
	Name          string         `yaml:"name"`
	SourcePath    string         `yaml:"sourcePath"`
	Image         string         `yaml:"image"`
	Chart         string         `yaml:"chart"`
	Namespace     string         `yaml:"namespace"`
	PodSelector   string         `yaml:"podSelector"`
	Values        map[string]any `yaml:"values"`
	UpstreamTests []string       `yaml:"upstreamTests"`
	ClaimConfig   *ClaimConfig   `yaml:"claimConfig"`
	SriovPolicy   *SriovPolicy   `yaml:"sriovPolicy"`
}

// SriovPolicy describes the raw spec of the SR-IOV driver's
// SriovResourcePolicy. The harness creates this namespaced resource after the
// driver chart is installed and removes it during cleanup.
type SriovPolicy struct {
	Name      string         `yaml:"name"`
	Namespace string         `yaml:"namespace"`
	Spec      map[string]any `yaml:"spec"`
}

// TestPlan selects live, namespace-scoped DRA scenarios to run after driver
// setup. It is intentionally separate from Workload: the regular workload is
// a smoke check, while a plan can create several claims and expected-pending
// consumers.
type TestPlan struct {
	Profile       string               `yaml:"profile"`
	Scenarios     []string             `yaml:"scenarios"`
	WorkloadImage string               `yaml:"workloadImage"`
	Selector      string               `yaml:"selector"`
	ClaimConfig   *ClaimConfig         `yaml:"claimConfig"`
	Lifecycle     TestPlanLifecycle    `yaml:"lifecycle"`
	Verification  TestPlanVerification `yaml:"verification"`
	Topology      []TopologyCase       `yaml:"topology"`
}

// TestPlanLifecycle controls mutations outside the normal harness install.
// Existing-driver runs require these explicit opt-ins.
type TestPlanLifecycle struct {
	AllowWorkloads bool `yaml:"allowWorkloads"`
	AllowRestart   bool `yaml:"allowRestart"`
}

// TestPlanVerification configures the external topology verification scripts.
type TestPlanVerification struct {
	ScriptsDir         string     `yaml:"scriptsDir"`
	ExpectedRepoCommit string     `yaml:"expectedRepoCommit"`
	EvidenceDir        string     `yaml:"evidenceDir"`
	Commands           [][]string `yaml:"commands"`
}

// TopologyCase describes one multi-request ResourceClaim scenario. Device
// classes may refer to drivers installed outside this harness.
type TopologyCase struct {
	Name           string            `yaml:"name"`
	UseTemplate    bool              `yaml:"useTemplate"`
	Requests       []TopologyRequest `yaml:"requests"`
	MatchAttribute string            `yaml:"matchAttribute"`
	Expected       string            `yaml:"expected"`
}

// TopologyRequest is one named request in a TopologyCase.
type TopologyRequest struct {
	Name        string `yaml:"name"`
	DeviceClass string `yaml:"deviceClass"`
	Count       int64  `yaml:"count"`
	Selector    string `yaml:"selector"`
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
	if c.TestPlan != nil {
		if c.TestPlan.Verification.ScriptsDir != "" && !filepath.IsAbs(c.TestPlan.Verification.ScriptsDir) {
			c.TestPlan.Verification.ScriptsDir = filepath.Join(base, c.TestPlan.Verification.ScriptsDir)
		}
		if c.TestPlan.Verification.EvidenceDir != "" && !filepath.IsAbs(c.TestPlan.Verification.EvidenceDir) {
			c.TestPlan.Verification.EvidenceDir = filepath.Join(base, c.TestPlan.Verification.EvidenceDir)
		}
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
	if c.Workload == "" {
		c.Workload = WorkloadPod
	}
	if c.Workload != WorkloadPod && c.Workload != WorkloadKubeVirt {
		problems = append(problems, fmt.Errorf("unsupported workload %q (use %q or %q)", c.Workload, WorkloadPod, WorkloadKubeVirt))
	}
	if c.Workload == WorkloadKubeVirt {
		if c.KubeVirt == nil {
			problems = append(problems, errors.New("kubevirt workload requires kubevirt configuration"))
		} else {
			if c.KubeVirt.HoldAfterReadySeconds < 0 {
				problems = append(problems, errors.New("kubevirt holdAfterReadySeconds must not be negative"))
			}
			if c.KubeVirt.DeviceCount < 0 {
				problems = append(problems, errors.New("kubevirt deviceCount must not be negative"))
			}
			if c.KubeVirt.Image == "" {
				problems = append(problems, errors.New("kubevirt workload requires image"))
			}
			if c.KubeVirt.CPUModel != "" && c.KubeVirt.CPUModel != "host-model" && c.KubeVirt.CPUModel != "host-passthrough" {
				problems = append(problems, fmt.Errorf("unsupported kubevirt cpuModel %q (use host-model or host-passthrough)", c.KubeVirt.CPUModel))
			}
			if c.KubeVirt.NetworkBinding != "" && c.KubeVirt.NetworkBinding != "bridge" && c.KubeVirt.NetworkBinding != "masquerade" {
				problems = append(problems, fmt.Errorf("unsupported kubevirt networkBinding %q (use bridge or masquerade)", c.KubeVirt.NetworkBinding))
			}
			if c.KubeVirt.AllowExisting && !c.Existing {
				problems = append(problems, errors.New("kubevirt allowExisting requires existing: true"))
			}
			if c.Existing && c.Workload == WorkloadKubeVirt && !c.KubeVirt.AllowExisting {
				problems = append(problems, errors.New("existing kubevirt workload requires kubevirt.allowExisting: true"))
			}
			if c.KubeVirt.Attachment == "" {
				c.KubeVirt.Attachment = KubeVirtAttachmentGPU
				if len(c.Drivers) == 1 {
					if adapter, err := driver.Get(c.Drivers[0].Name); err == nil {
						if device, ok := driver.KubeVirtDeviceFor(adapter); ok && device.Attachment != "" {
							c.KubeVirt.Attachment = device.Attachment
						}
					}
				}
			}
			switch c.KubeVirt.Attachment {
			case KubeVirtAttachmentGPU, KubeVirtAttachmentHostDevice, KubeVirtAttachmentCPU, KubeVirtAttachmentNetwork:
			default:
				problems = append(problems, fmt.Errorf("unsupported kubevirt attachment %q (use gpu, hostDevice, cpu, or network)", c.KubeVirt.Attachment))
			}
			if c.KubeVirt.DeviceCount > 1 && c.KubeVirt.Attachment != KubeVirtAttachmentGPU && c.KubeVirt.Attachment != KubeVirtAttachmentHostDevice {
				problems = append(problems, fmt.Errorf("kubevirt deviceCount greater than one is only supported for gpu or hostDevice attachments"))
			}
			if c.KubeVirt.Namespace != "" && len(validation.IsDNS1123Label(c.KubeVirt.Namespace)) > 0 {
				problems = append(problems, errors.New("kubevirt namespace must be a DNS label"))
			}
			if c.KubeVirt.Attachment == KubeVirtAttachmentNetwork && c.KubeVirt.DeviceName != "" {
				if len(validation.IsDNS1123Label(c.KubeVirt.DeviceName)) > 0 || c.KubeVirt.DeviceName == "default" {
					problems = append(problems, errors.New("kubevirt network deviceName must be a DNS label other than default"))
				}
			}
			if c.KubeVirt.ClaimConfig != nil {
				claim := c.KubeVirt.ClaimConfig
				if claim.Driver != "" && len(validation.IsDNS1123Subdomain(claim.Driver)) > 0 {
					problems = append(problems, errors.New("kubevirt claimConfig driver must be a DNS subdomain"))
				}
				if claim.Driver == "" && len(claim.Parameters) > 0 {
					problems = append(problems, errors.New("kubevirt claimConfig parameters require driver"))
				}
				if claim.Driver != "" && len(claim.Parameters) == 0 {
					problems = append(problems, errors.New("kubevirt claimConfig driver requires parameters"))
				}
				for i, request := range claim.Requests {
					if request == "" || len(validation.IsDNS1123Label(request)) > 0 {
						problems = append(problems, fmt.Errorf("kubevirt claimConfig request[%d] must be a DNS label", i))
					}
				}
				for name, value := range claim.Capacity {
					if len(validation.IsQualifiedName(name)) > 0 {
						problems = append(problems, fmt.Errorf("kubevirt claimConfig capacity key %q must be a qualified name", name))
						continue
					}
					quantity, err := resource.ParseQuantity(fmt.Sprint(value))
					if err != nil {
						problems = append(problems, fmt.Errorf("kubevirt claimConfig capacity %q must be a resource quantity: %v", name, err))
					} else if quantity.Sign() <= 0 {
						problems = append(problems, fmt.Errorf("kubevirt claimConfig capacity %q must be greater than zero", name))
					}
				}
			}
			if c.KubeVirt.Guest != nil {
				guest := c.KubeVirt.Guest
				if guest.Username == "" || guest.PrivateKeySecret == "" || guest.Command == "" {
					problems = append(problems, errors.New("kubevirt guest requires username, privateKeySecret, and command"))
				}
				if guest.PrivateKeyKey == "" {
					guest.PrivateKeyKey = "id_rsa"
				}
				if guest.ExpectedOutput == "" {
					guest.ExpectedOutput = "PASS"
				}
			}
			if c.KubeVirt.Namespace == "" && (c.KubeVirt.CloudInitSecret != "" || c.KubeVirt.Guest != nil) {
				problems = append(problems, errors.New("kubevirt namespace is required when cloud-init or guest verification is configured"))
			}
			if len(c.Drivers) == 0 {
				problems = append(problems, errors.New("kubevirt workload requires at least one driver"))
			} else {
				for _, d := range c.Drivers {
					adapter, err := driver.Get(d.Name)
					if err == nil {
						device, supported := driver.KubeVirtDeviceFor(adapter)
						if !supported {
							problems = append(problems, fmt.Errorf("kubevirt workload does not support driver %q", d.Name))
						} else if !kubeVirtAttachmentCompatible(c.KubeVirt.Attachment, device.Attachment) {
							problems = append(problems, fmt.Errorf("kubevirt attachment %q is incompatible with driver %q", c.KubeVirt.Attachment, d.Name))
						}
					}
				}
			}
		}
	} else if c.KubeVirt != nil {
		problems = append(problems, errors.New("kubevirt configuration requires workload: kubevirt"))
	}
	if c.TestPlan != nil {
		if c.Workload == WorkloadKubeVirt {
			problems = append(problems, errors.New("testPlan cannot be combined with workload: kubevirt"))
		}
		if c.Preflight {
			problems = append(problems, errors.New("testPlan cannot run in preflight mode"))
		}
		if len(c.Drivers) == 0 {
			problems = append(problems, errors.New("testPlan requires at least one driver"))
		}
		if c.TestPlan.Profile == "" {
			problems = append(problems, errors.New("testPlan requires profile"))
		}
		if len(c.TestPlan.Scenarios) == 0 {
			problems = append(problems, errors.New("testPlan requires at least one scenario"))
		}
		if c.TestPlan.ClaimConfig != nil {
			claim := c.TestPlan.ClaimConfig
			if claim.Driver == "" {
				problems = append(problems, errors.New("testPlan claimConfig requires driver"))
			} else if len(validation.IsDNS1123Subdomain(claim.Driver)) > 0 {
				problems = append(problems, errors.New("testPlan claimConfig driver must be a DNS subdomain"))
			}
			if len(claim.Parameters) == 0 {
				problems = append(problems, errors.New("testPlan claimConfig requires parameters"))
			}
			for i, request := range claim.Requests {
				if request == "" || len(validation.IsDNS1123Label(request)) > 0 {
					problems = append(problems, fmt.Errorf("testPlan claimConfig request[%d] must be a DNS label", i))
				}
			}
		}
		validScenarios := map[string]bool{
			"resource-slices": true, "counters": true, "sibling-exclusion": true,
			"sibling-exclusion-reverse": true, "alternate-device": true,
			"capacity": true, "release": true, "release-orders": true, "restart": true, "restart-active": true, "topology": true,
		}
		hasAMDDriver := false
		for _, driver := range c.Drivers {
			if driver.Name == "amd" {
				hasAMDDriver = true
				break
			}
		}
		for _, scenario := range c.TestPlan.Scenarios {
			if !validScenarios[scenario] {
				problems = append(problems, fmt.Errorf("unsupported testPlan scenario %q", scenario))
			}
			if (scenario == "sibling-exclusion" || scenario == "capacity") && !hasAMDDriver {
				problems = append(problems, fmt.Errorf("testPlan scenario %q requires the amd driver", scenario))
			}
			if scenario == "topology" && len(c.TestPlan.Topology) == 0 {
				problems = append(problems, errors.New("testPlan topology scenario requires topology cases"))
			}
		}
		if c.TestPlan.Lifecycle.AllowRestart && !c.TestPlan.Lifecycle.AllowWorkloads {
			problems = append(problems, errors.New("testPlan lifecycle allowRestart requires allowWorkloads"))
		}
		if !c.TestPlan.Lifecycle.AllowWorkloads {
			for _, scenario := range c.TestPlan.Scenarios {
				if scenario != "resource-slices" && scenario != "counters" {
					problems = append(problems, fmt.Errorf("testPlan scenario %q requires lifecycle.allowWorkloads: true", scenario))
				}
			}
		}
		for i, topology := range c.TestPlan.Topology {
			if topology.Name == "" {
				problems = append(problems, fmt.Errorf("testPlan topology[%d] requires name", i))
			} else if len(validation.IsDNS1123Label(topology.Name)) > 0 {
				problems = append(problems, fmt.Errorf("testPlan topology %q name must be a DNS label", topology.Name))
			}
			if len(topology.Requests) < 2 {
				problems = append(problems, fmt.Errorf("testPlan topology %q requires at least two requests", topology.Name))
			}
			if topology.Expected != "" && topology.Expected != "success" && topology.Expected != "pending" {
				problems = append(problems, fmt.Errorf("testPlan topology %q expected must be success or pending", topology.Name))
			}
			if topology.MatchAttribute != "" && len(validation.IsQualifiedName(topology.MatchAttribute)) > 0 {
				problems = append(problems, fmt.Errorf("testPlan topology %q matchAttribute must be a qualified name", topology.Name))
			}
			seenRequests := map[string]bool{}
			for _, request := range topology.Requests {
				if request.Name == "" || request.DeviceClass == "" {
					problems = append(problems, fmt.Errorf("testPlan topology %q requests require name and deviceClass", topology.Name))
				}
				if request.DeviceClass != "" && len(validation.IsDNS1123Subdomain(request.DeviceClass)) > 0 {
					problems = append(problems, fmt.Errorf("testPlan topology %q request %q deviceClass must be a DNS subdomain", topology.Name, request.Name))
				}
				if request.Name != "" && len(validation.IsDNS1123Label(request.Name)) > 0 {
					problems = append(problems, fmt.Errorf("testPlan topology %q request %q name must be a DNS label", topology.Name, request.Name))
				}
				if request.Count < 0 {
					problems = append(problems, fmt.Errorf("testPlan topology %q request %q count must not be negative", topology.Name, request.Name))
				}
				if seenRequests[request.Name] {
					problems = append(problems, fmt.Errorf("testPlan topology %q has duplicate request %q", topology.Name, request.Name))
				}
				seenRequests[request.Name] = true
			}
		}
		if c.TestPlan.Verification.ExpectedRepoCommit != "" && c.TestPlan.Verification.ScriptsDir == "" {
			problems = append(problems, errors.New("testPlan verification expectedRepoCommit requires scriptsDir"))
		}
		if c.TestPlan.Verification.ScriptsDir != "" && c.TestPlan.Verification.EvidenceDir == "" {
			problems = append(problems, errors.New("testPlan verification scriptsDir requires evidenceDir"))
		}
		if len(c.TestPlan.Verification.Commands) > 0 && c.TestPlan.Verification.ScriptsDir == "" {
			problems = append(problems, errors.New("testPlan verification commands require scriptsDir"))
		}
		for i, command := range c.TestPlan.Verification.Commands {
			if len(command) == 0 || strings.TrimSpace(command[0]) == "" {
				problems = append(problems, fmt.Errorf("testPlan verification command[%d] requires an executable", i))
			}
		}
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
			if d.PodSelector == "" && c.TestPlan != nil && c.TestPlan.Lifecycle.AllowRestart {
				problems = append(problems, fmt.Errorf("existing driver %q requires podSelector for testPlan restart", d.Name))
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
		if d.ClaimConfig != nil {
			claim := d.ClaimConfig
			if claim.Driver == "" {
				problems = append(problems, fmt.Errorf("driver %q claimConfig requires driver", d.Name))
			} else if len(validation.IsDNS1123Subdomain(claim.Driver)) > 0 {
				problems = append(problems, fmt.Errorf("driver %q claimConfig driver must be a DNS subdomain", d.Name))
			}
			if len(claim.Parameters) == 0 {
				problems = append(problems, fmt.Errorf("driver %q claimConfig requires parameters", d.Name))
			}
			for i, request := range claim.Requests {
				if request == "" || len(validation.IsDNS1123Label(request)) > 0 {
					problems = append(problems, fmt.Errorf("driver %q claimConfig request[%d] must be a DNS label", d.Name, i))
				}
			}
		}
		if d.SriovPolicy != nil {
			if d.Name != "sriov" {
				problems = append(problems, fmt.Errorf("driver %q cannot configure sriovPolicy", d.Name))
			}
			if c.Existing {
				problems = append(problems, errors.New("existing mode cannot create sriovPolicy"))
			}
			if d.SriovPolicy.Name != "" && len(validation.IsDNS1123Subdomain(d.SriovPolicy.Name)) > 0 {
				problems = append(problems, errors.New("sriovPolicy name must be a DNS subdomain"))
			}
			if d.SriovPolicy.Namespace != "" && len(validation.IsDNS1123Label(d.SriovPolicy.Namespace)) > 0 {
				problems = append(problems, errors.New("sriovPolicy namespace must be a DNS label"))
			}
			if d.Namespace != "" && d.SriovPolicy.Namespace != "" && d.Namespace != d.SriovPolicy.Namespace {
				problems = append(problems, errors.New("sriovPolicy namespace must match driver namespace"))
			}
			if len(d.SriovPolicy.Spec) == 0 {
				problems = append(problems, errors.New("sriovPolicy requires spec"))
			}
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

func kubeVirtAttachmentCompatible(attachment, deviceAttachment string) bool {
	switch attachment {
	case KubeVirtAttachmentGPU, KubeVirtAttachmentHostDevice:
		return deviceAttachment == KubeVirtAttachmentGPU
	case KubeVirtAttachmentCPU:
		return deviceAttachment == KubeVirtAttachmentCPU
	case KubeVirtAttachmentNetwork:
		return deviceAttachment == KubeVirtAttachmentNetwork
	default:
		return false
	}
}

// ShouldCleanup defaults to true.
func (c *Config) ShouldCleanup() bool { return c.Cleanup == nil || *c.Cleanup }
