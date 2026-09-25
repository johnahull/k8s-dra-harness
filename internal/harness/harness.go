// Package harness orchestrates a run against a user-provided cluster.
package harness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/johnahull/k8s-dra-harness/internal/amdoperator"
	"github.com/johnahull/k8s-dra-harness/internal/driver"
	"github.com/johnahull/k8s-dra-harness/internal/runconfig"
	"github.com/johnahull/k8s-dra-harness/pkg/clients"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
)

// Installed is one driver installed by this run.
type Installed struct {
	Adapter   driver.Adapter
	Config    runconfig.Driver
	Image     string
	Chart     string
	Namespace string
	Release   string
}

// Runner owns only namespaces and Helm releases created during this run.
type Runner struct {
	Config           *runconfig.Config
	Client           *clients.Settings
	ID               string
	Drivers          []Installed
	releases         releaseManager
	namespaces       []string
	bundlePackage    string
	bundleNamespace  string
	deviceConfigName string
	openshift        bool
}

// New validates the configuration and connects to the cluster.
func New(c *runconfig.Config) (*Runner, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if c.Namespace == "" {
		c.Namespace = "dra-harness"
	}
	client, err := clients.New("")
	if err != nil {
		return nil, err
	}
	return &Runner{Config: c, Client: client, ID: rand.String(8)}, nil
}

// Preflight checks DRA API availability and collisions before builds or installs.
func (r *Runner) Preflight(ctx context.Context) error {
	if len(r.Config.Drivers) > 0 {
		if _, err := r.Client.Discovery.ServerResourcesForGroupVersion("resource.k8s.io/v1"); err != nil {
			return fmt.Errorf("cluster must serve resource.k8s.io/v1: %w", err)
		}
	}
	groups, err := r.Client.Discovery.ServerGroups()
	if err != nil {
		return fmt.Errorf("discovering cluster APIs: %w", err)
	}
	for _, g := range groups.Groups {
		if g.Name == "config.openshift.io" {
			r.openshift = true
			break
		}
	}
	if r.openshift {
		if _, err := exec.LookPath("oc"); err != nil {
			return fmt.Errorf("oc is required for OpenShift SCC setup: %w", err)
		}
	}
	if r.Config.Operator != nil {
		if r.Config.Operator.Bundle != "" && !r.openshift {
			return errors.New("amdOperator.bundle requires OpenShift; use chart on Kubernetes")
		}
		if r.Config.Operator.Chart != "" && r.openshift {
			return errors.New("amdOperator.chart requires Kubernetes; use bundle on OpenShift")
		}
	}
	for _, d := range r.Config.Drivers {
		a, _ := driver.Get(d.Name)
		_, err := r.Client.K8s.ResourceV1().DeviceClasses().Get(ctx, a.DeviceClass(), metav1.GetOptions{})
		if err == nil {
			return fmt.Errorf("DeviceClass %q already exists; refusing to replace an existing driver", a.DeviceClass())
		}
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("checking DeviceClass %q: %w", a.DeviceClass(), err)
		}
		ns := r.namespace(d.Namespace)
		name := "dra-" + d.Name + "-" + r.ID
		if err := r.releases.requireAbsent(ctx, release{ns, name}); err != nil {
			return err
		}
	}
	if r.Config.Operator != nil {
		if err := amdoperator.CheckDeviceConfigs(ctx, r.Client.Config); err != nil {
			return err
		}
		if r.Config.Operator.Bundle != "" {
			if _, err := exec.LookPath("operator-sdk"); err != nil {
				return fmt.Errorf("operator-sdk is required for bundle installs: %w", err)
			}
		} else if err := r.releases.requireAbsent(ctx, release{r.namespace(r.Config.Operator.Namespace), "amd-operator-" + r.ID}); err != nil {
			return err
		}
	}
	return nil
}

// Start builds every selected checkout, then installs the optional operator and
// drivers. Call Cleanup if Start returns an error after partial installation.
func (r *Runner) Start(ctx context.Context) error {
	if err := r.Preflight(ctx); err != nil {
		return err
	}
	for _, d := range r.Config.Drivers {
		a, _ := driver.Get(d.Name)
		image, chart, err := driver.Resolve(ctx, a, driver.Source{Path: d.SourcePath, Image: d.Image, Chart: d.Chart}, r.Config.Registry, r.ID)
		if err != nil {
			return err
		}
		if _, _, err = driver.SplitImage(image); err != nil {
			return err
		}
		r.Drivers = append(r.Drivers, Installed{Adapter: a, Config: d, Image: image, Chart: chart,
			Namespace: r.namespace(d.Namespace), Release: "dra-" + d.Name + "-" + r.ID})
	}
	if err := r.ensureNamespace(ctx, r.WorkloadNamespace()); err != nil {
		return err
	}
	if r.Config.Operator != nil {
		if err := r.installOperator(ctx); err != nil {
			return err
		}
	}
	for _, d := range r.Drivers {
		if err := r.ensureNamespace(ctx, d.Namespace); err != nil {
			return err
		}
		values, err := d.Adapter.Values(d.Config.Values, d.Image)
		if err != nil {
			return fmt.Errorf("preparing %s values: %w", d.Config.Name, err)
		}
		if err := r.releases.installRelease(ctx, release{d.Namespace, d.Release}, d.Chart, values); err != nil {
			return err
		}
	}
	return nil
}

// WorkloadNamespace is unique to this invocation.
func (r *Runner) WorkloadNamespace() string { return r.Config.Namespace + "-" + r.ID + "-workload" }

func (r *Runner) namespace(configured string) string {
	if configured != "" {
		return configured
	}
	return r.Config.Namespace + "-" + r.ID
}

func (r *Runner) ensureNamespace(ctx context.Context, name string) error {
	_, err := r.Client.K8s.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("checking namespace %s: %w", name, err)
	}
	_, err = r.Client.K8s.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{"pod-security.kubernetes.io/enforce": "privileged", "app.kubernetes.io/managed-by": "k8s-dra-harness"}}}, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("creating namespace %s: %w", name, err)
	}
	r.namespaces = append(r.namespaces, name)
	if r.openshift && name != r.WorkloadNamespace() {
		if err := run(ctx, "oc", "adm", "policy", "add-scc-to-group", "privileged", "system:serviceaccounts:"+name, "--namespace", name); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) installOperator(ctx context.Context) error {
	o := r.Config.Operator
	ns := r.namespace(o.Namespace)
	if err := r.ensureNamespace(ctx, ns); err != nil {
		return err
	}
	if o.Bundle != "" {
		if o.Image != "" {
			return errors.New("bundle installs must carry their operator image; amdOperator.image is unsupported")
		}
		r.bundlePackage, r.bundleNamespace = o.Package, ns
		if err := run(ctx, "operator-sdk", "run", "bundle", o.Bundle, "--namespace", ns, "--timeout", "10m"); err != nil {
			return err
		}
		spec := amdoperator.DeviceConfigSpec(o.DeviceConfig, amdoperator.HasStandaloneDriver(r.Config.Drivers))
		return r.createDeviceConfig(ctx, ns, spec)
	}
	values, err := amdoperator.ChartValues(o, amdoperator.HasStandaloneDriver(r.Config.Drivers))
	if err != nil {
		return err
	}
	name := "amd-operator-" + r.ID
	if err := r.releases.installRelease(ctx, release{ns, name}, o.Chart, values); err != nil {
		return err
	}
	return nil
}

// Cleanup releases resources in reverse order, continuing after errors.
func (r *Runner) Cleanup(ctx context.Context) error {
	var problems []error
	releaseErr := r.releases.cleanup(ctx)
	if releaseErr != nil {
		problems = append(problems, releaseErr)
	}
	if r.bundlePackage != "" {
		if r.deviceConfigName != "" {
			if err := r.deleteDeviceConfig(ctx); err != nil {
				problems = append(problems, err)
			}
		}
		deleteGroups := "--delete-operator-groups=false"
		for _, ns := range r.namespaces {
			if ns == r.bundleNamespace {
				deleteGroups = "--delete-operator-groups=true"
				break
			}
		}
		if err := run(ctx, "operator-sdk", "cleanup", r.bundlePackage, "--namespace", r.bundleNamespace, "--delete-all=false", "--delete-crds=false", deleteGroups); err != nil {
			problems = append(problems, err)
		}
	}
	// Leave owned namespaces available for a retry if any resource cleanup
	// failed, including an unknown Helm state.
	if len(problems) > 0 {
		return errors.Join(problems...)
	}
	for i := len(r.namespaces) - 1; i >= 0; i-- {
		name := r.namespaces[i]
		if err := r.Client.K8s.CoreV1().Namespaces().Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			problems = append(problems, fmt.Errorf("deleting namespace %s: %w", name, err))
		}
	}
	return errors.Join(problems...)
}

func (r *Runner) createDeviceConfig(ctx context.Context, namespace string, spec map[string]any) error {
	name := "dra-harness-" + r.ID
	if err := amdoperator.CreateDeviceConfig(ctx, r.Client.Config, namespace, name, spec); err != nil {
		return err
	}
	r.deviceConfigName = name
	return nil
}

func (r *Runner) deleteDeviceConfig(ctx context.Context) error {
	return amdoperator.DeleteDeviceConfig(ctx, r.Client.Config, r.bundleNamespace, r.deviceConfigName)
}

func helmInstall(ctx context.Context, name, chart, namespace string, values map[string]any) error {
	if err := prepareChart(ctx, chart); err != nil {
		return err
	}
	data, err := yaml.Marshal(values)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp("", "dra-harness-values-*.yaml")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return run(ctx, "helm", "install", name, chart, "--namespace", namespace, "--wait", "--timeout", "10m", "--values", f.Name())
}

func prepareChart(ctx context.Context, chart string) error {
	data, err := os.ReadFile(filepath.Join(chart, "Chart.yaml"))
	if os.IsNotExist(err) {
		return nil // Helm handles repository and OCI charts.
	}
	if err != nil {
		return fmt.Errorf("reading chart metadata: %w", err)
	}
	var metadata struct {
		Dependencies []struct {
			Name       string `yaml:"name"`
			Repository string `yaml:"repository"`
		} `yaml:"dependencies"`
	}
	if err := yaml.Unmarshal(data, &metadata); err != nil {
		return fmt.Errorf("parsing chart metadata: %w", err)
	}
	if len(metadata.Dependencies) == 0 {
		return nil
	}
	temp, err := os.MkdirTemp("", "dra-harness-helm-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(temp) }()
	env := append(os.Environ(), "HELM_REPOSITORY_CONFIG="+filepath.Join(temp, "repositories.yaml"), "HELM_REPOSITORY_CACHE="+filepath.Join(temp, "cache"))
	for i, dependency := range metadata.Dependencies {
		if strings.HasPrefix(dependency.Repository, "https://") || strings.HasPrefix(dependency.Repository, "http://") {
			if err := runEnv(ctx, env, "helm", "repo", "add", fmt.Sprintf("dra-harness-%d", i), dependency.Repository); err != nil {
				return err
			}
		}
	}
	return runEnv(ctx, env, "helm", "dependency", "build", chart)
}

func run(ctx context.Context, name string, args ...string) error {
	return runEnv(ctx, os.Environ(), name, args...)
}

func runEnv(ctx context.Context, env []string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

// RunUpstreamTests invokes a checkout's own e2e command against this run's
// cluster after deployment. The command is an argv list, never a shell script.
func (r *Runner) RunUpstreamTests(ctx context.Context, d Installed) error {
	args := d.Config.UpstreamTests
	if len(args) == 0 {
		return nil
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = d.Config.SourcePath
	cmd.Env = append(os.Environ(), "DRA_HARNESS_IMAGE="+d.Image, "DRA_HARNESS_NAMESPACE="+d.Namespace)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s upstream tests (%s): %w", d.Config.Name, strings.Join(args, " "), err)
	}
	return nil
}
