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

	"github.com/johnahull/k8s-dra-harness/internal/driver"
	"github.com/johnahull/k8s-dra-harness/internal/runconfig"
	"github.com/johnahull/k8s-dra-harness/pkg/clients"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/dynamic"
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

type release struct{ namespace, name string }

const (
	amdName = "amd"
	cpuName = "cpu"
)

// Runner owns only namespaces and Helm releases created during this run.
type Runner struct {
	Config           *runconfig.Config
	Client           *clients.Settings
	ID               string
	Drivers          []Installed
	releases         []release
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
		if err := availableRelease(ctx, ns, name); err != nil {
			return err
		}
	}
	if r.Config.Operator != nil {
		dcClient, err := dynamic.NewForConfig(r.Client.Config)
		if err != nil {
			return fmt.Errorf("creating AMD API client: %w", err)
		}
		configs, err := dcClient.Resource(deviceConfigGVR).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("checking existing DeviceConfigs: %w", err)
		}
		if err == nil && len(configs.Items) > 0 {
			return errors.New("AMD DeviceConfigs already exist; refusing to install another operator")
		}
		if r.Config.Operator.Bundle != "" {
			if _, err := exec.LookPath("operator-sdk"); err != nil {
				return fmt.Errorf("operator-sdk is required for bundle installs: %w", err)
			}
		} else if err := availableRelease(ctx, r.namespace(r.Config.Operator.Namespace), "amd-operator-"+r.ID); err != nil {
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
		image, chart, err := driver.Resolve(ctx, a, d, r.Config.Registry, r.ID)
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
		repo, tag, _ := driver.SplitImage(d.Image)
		values := cloneValues(d.Config.Values)
		values["image"] = mergeImage(values["image"], repo, tag)
		if d.Config.Name == cpuName {
			cpuConfig, _ := values["driverConfig"].(map[string]any)
			if cpuConfig == nil {
				cpuConfig = map[string]any{}
			}
			if _, ok := cpuConfig["cpuDeviceMode"]; !ok {
				cpuConfig["cpuDeviceMode"] = "individual"
			}
			values["driverConfig"] = cpuConfig
		}
		r.releases = append(r.releases, release{d.Namespace, d.Release})
		if err := helmInstall(ctx, d.Release, d.Chart, d.Namespace, values); err != nil {
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
		spec := cloneValues(o.DeviceConfig)
		for _, d := range r.Config.Drivers {
			if d.Name == amdName {
				setValue(spec, false, "devicePlugin", "enableDevicePlugin")
				setValue(spec, false, "draDriver", "enable")
				break
			}
		}
		return r.createDeviceConfig(ctx, ns, spec)
	}
	values := cloneValues(o.Values)
	for _, d := range r.Config.Drivers {
		if d.Name == amdName {
			setValue(values, false, "deviceConfig", "spec", "devicePlugin", "enableDevicePlugin")
			setValue(values, false, "deviceConfig", "spec", "draDriver", "enable")
			break
		}
	}
	if o.Image != "" {
		repo, tag, err := driver.SplitImage(o.Image)
		if err != nil {
			return err
		}
		cm, _ := values["controllerManager"].(map[string]any)
		if cm == nil {
			cm = map[string]any{}
		}
		manager, _ := cm["manager"].(map[string]any)
		if manager == nil {
			manager = map[string]any{}
		}
		manager["image"] = map[string]any{"repository": repo, "tag": tag}
		cm["manager"] = manager
		values["controllerManager"] = cm
	}
	name := "amd-operator-" + r.ID
	r.releases = append(r.releases, release{ns, name})
	if err := helmInstall(ctx, name, o.Chart, ns, values); err != nil {
		return err
	}
	return nil
}

// Cleanup releases resources in reverse order, continuing after errors.
func (r *Runner) Cleanup(ctx context.Context) error {
	var problems []error
	for i := len(r.releases) - 1; i >= 0; i-- {
		rel := r.releases[i]
		if err := availableRelease(ctx, rel.namespace, rel.name); err == nil {
			continue
		}
		if err := run(ctx, "helm", "uninstall", rel.name, "--namespace", rel.namespace, "--wait", "--timeout", "10m"); err != nil {
			problems = append(problems, err)
		}
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
	for i := len(r.namespaces) - 1; i >= 0; i-- {
		name := r.namespaces[i]
		if err := r.Client.K8s.CoreV1().Namespaces().Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			problems = append(problems, fmt.Errorf("deleting namespace %s: %w", name, err))
		}
	}
	return errors.Join(problems...)
}

var deviceConfigGVR = schema.GroupVersionResource{Group: "amd.com", Version: "v1alpha1", Resource: "deviceconfigs"}

func (r *Runner) createDeviceConfig(ctx context.Context, namespace string, spec map[string]any) error {
	client, err := dynamic.NewForConfig(r.Client.Config)
	if err != nil {
		return err
	}
	name := "dra-harness-" + r.ID
	obj := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "amd.com/v1alpha1", "kind": "DeviceConfig", "metadata": map[string]any{"name": name, "namespace": namespace}, "spec": spec}}
	if _, err := client.Resource(deviceConfigGVR).Namespace(namespace).Create(ctx, obj, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("creating DeviceConfig: %w", err)
	}
	r.deviceConfigName = name
	return nil
}

func (r *Runner) deleteDeviceConfig(ctx context.Context) error {
	client, err := dynamic.NewForConfig(r.Client.Config)
	if err != nil {
		return err
	}
	if err := client.Resource(deviceConfigGVR).Namespace(r.bundleNamespace).Delete(ctx, r.deviceConfigName, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("deleting DeviceConfig %s: %w", r.deviceConfigName, err)
	}
	return nil
}

func cloneValues(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		if nested, ok := v.(map[string]any); ok {
			out[k] = cloneValues(nested)
		} else {
			out[k] = v
		}
	}
	return out
}

func setValue(values map[string]any, value any, path ...string) {
	current := values
	for _, key := range path[:len(path)-1] {
		next, _ := current[key].(map[string]any)
		if next == nil {
			next = map[string]any{}
			current[key] = next
		}
		current = next
	}
	current[path[len(path)-1]] = value
}

func mergeImage(existing any, repo, tag string) map[string]any {
	out, _ := existing.(map[string]any)
	if out == nil {
		out = map[string]any{}
	}
	out["repository"], out["tag"] = repo, tag
	return out
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

func availableRelease(ctx context.Context, namespace, name string) error {
	cmd := exec.CommandContext(ctx, "helm", "list", "--all", "--namespace", namespace, "--filter", "^"+name+"$", "--short")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("checking Helm release: %w: %s", err, out)
	}
	if strings.TrimSpace(string(out)) != "" {
		return fmt.Errorf("helm release %s/%s already exists", namespace, name)
	}
	return nil
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
