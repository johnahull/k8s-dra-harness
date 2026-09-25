// Package nvidiaoperator holds the NVIDIA GPU Operator policy used by the harness.
package nvidiaoperator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/johnahull/k8s-dra-harness/internal/driver"
	"github.com/johnahull/k8s-dra-harness/internal/runconfig"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const defaultDriverInstallDir = "/run/nvidia/driver"

var (
	clusterPolicyGVR = schema.GroupVersionResource{Group: "nvidia.com", Version: "v1", Resource: "clusterpolicies"}
	gpuClusterGVR    = schema.GroupVersionResource{Group: "nvidia.com", Version: "v1alpha1", Resource: "gpuclusters"}
)

// HasStandaloneDriver reports whether the run installs the standalone NVIDIA
// GPU DRA plugin. The classic device plugin must be disabled in that case so
// both allocators do not advertise or bind the same GPUs.
func HasStandaloneDriver(drivers []runconfig.Driver) bool {
	for _, selected := range drivers {
		if selected.Name == "nvidia" && GPUResourcesEnabled(selected.Values) {
			return true
		}
	}
	return false
}

// GPUResourcesEnabled reports whether the NVIDIA DRA chart will deploy its GPU
// kubelet plugin. The chart defaults this feature to true.
func GPUResourcesEnabled(values map[string]any) bool {
	resources, _ := values["resources"].(map[string]any)
	gpus, _ := resources["gpus"].(map[string]any)
	if enabled, ok := gpus["enabled"].(bool); ok {
		return enabled
	}
	return true
}

// ChartValues prepares GPU Operator chart values. This harness uses the
// classic ClusterPolicy path because the standalone NVIDIA DRA chart owns DRA
// resources; the GPUCluster path would create a second DRA owner.
func ChartValues(o *runconfig.NVIDIAOperator, standaloneNVIDIA, openshift bool) (map[string]any, error) {
	values := cloneValues(o.Values)
	gpuCluster, _ := values["gpuCluster"].(map[string]any)
	if gpuCluster != nil {
		if enabled, ok := gpuCluster["deployCR"].(bool); ok && enabled {
			return nil, fmt.Errorf("nvidiaOperator.gpuCluster.deployCR is unsupported; use the standalone nvidia driver")
		}
	}
	setValue(values, false, "gpuCluster", "deployCR")
	setValue(values, true, "clusterPolicy", "deployCR")
	if standaloneNVIDIA {
		setValue(values, false, "devicePlugin", "enabled")
	}
	platform, ok := values["platform"].(map[string]any)
	if !ok {
		platform = map[string]any{}
		values["platform"] = platform
	}
	if _, ok := platform["openshift"]; !ok {
		platform["openshift"] = openshift
	}
	if o.Image != "" {
		repo, tag, err := driver.SplitImage(o.Image)
		if err != nil {
			return nil, err
		}
		image := repo
		if i := strings.LastIndex(repo, "/"); i >= 0 {
			setValue(values, repo[:i], "operator", "repository")
			image = repo[i+1:]
		}
		setValue(values, image, "operator", "image")
		setValue(values, tag, "operator", "version")
	}
	return values, nil
}

// DriverValues aligns the standalone DRA driver's host path with the GPU
// Operator. The operator-managed driver defaults to /run/nvidia/driver, while
// the standalone DRA chart defaults to /; letting those defaults diverge makes
// the DRA kubelet plugin unable to see the installed driver libraries.
func DriverValues(values map[string]any, o *runconfig.NVIDIAOperator) (map[string]any, error) {
	out := cloneValues(values)
	operatorValues := cloneValues(o.Values)
	driverConfig, _ := operatorValues["driver"].(map[string]any)
	if enabled, ok := driverConfig["enabled"].(bool); ok && !enabled {
		return out, nil
	}

	driverRoot := defaultDriverInstallDir
	hostPaths, _ := operatorValues["hostPaths"].(map[string]any)
	if raw, exists := hostPaths["driverInstallDir"]; exists {
		configured, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("nvidiaOperator.values.hostPaths.driverInstallDir must be a string")
		}
		if configured == "" {
			return nil, fmt.Errorf("nvidiaOperator.values.hostPaths.driverInstallDir must not be empty")
		}
		driverRoot = configured
	}
	if raw, exists := out["nvidiaDriverRoot"]; exists {
		configured, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("driver values nvidiaDriverRoot must be a string")
		}
		if configured == "" {
			return nil, fmt.Errorf("driver values nvidiaDriverRoot must not be empty")
		}
		if configured != driverRoot {
			return nil, fmt.Errorf("driver nvidiaDriverRoot %q conflicts with NVIDIA Operator hostPaths.driverInstallDir %q", configured, driverRoot)
		}
		return out, nil
	}
	setValue(out, driverRoot, "nvidiaDriverRoot")
	return out, nil
}

// CheckPolicies refuses to install a second GPU Operator over existing
// operator CRs, deployments, or the chart's fixed ClusterRole. A random Helm
// release name alone is not sufficient to make a second install safe.
func CheckPolicies(ctx context.Context, config *rest.Config) error {
	client, err := dynamic.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("creating NVIDIA API client: %w", err)
	}
	for _, gvr := range []schema.GroupVersionResource{clusterPolicyGVR, gpuClusterGVR} {
		objects, err := client.Resource(gvr).List(ctx, metav1.ListOptions{})
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("checking existing NVIDIA GPU Operator %s: %w", gvr.Resource, err)
		}
		if len(objects.Items) > 0 {
			return fmt.Errorf("NVIDIA GPU Operator resource %s/%s already exists; refusing to install another operator", gvr.Resource, objects.Items[0].GetName())
		}
	}
	clusterRoles := client.Resource(schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterroles"})
	if _, err := clusterRoles.Get(ctx, "gpu-operator", metav1.GetOptions{}); err == nil {
		return errors.New("NVIDIA GPU Operator ClusterRole gpu-operator already exists; refusing to install another operator")
	} else if !apierrors.IsNotFound(err) {
		return fmt.Errorf("checking NVIDIA GPU Operator ClusterRole gpu-operator: %w", err)
	}
	deployments := client.Resource(schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}).Namespace(metav1.NamespaceAll)
	objects, err := deployments.List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/component=gpu-operator"})
	if err != nil {
		return fmt.Errorf("checking existing NVIDIA GPU Operator deployments: %w", err)
	}
	if len(objects.Items) > 0 {
		return fmt.Errorf("NVIDIA GPU Operator deployment %s/%s already exists; refusing to install another operator", objects.Items[0].GetNamespace(), objects.Items[0].GetName())
	}
	return nil
}

// CheckStandardDevicePlugin refuses to install the GPU DRA resources while a
// standard NVIDIA device plugin is already advertising nvidia.com/gpu.
func CheckStandardDevicePlugin(ctx context.Context, client kubernetes.Interface) error {
	nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("checking NVIDIA extended resources: %w", err)
	}
	for _, node := range nodes.Items {
		if quantity := node.Status.Allocatable[corev1.ResourceName("nvidia.com/gpu")]; quantity.Value() > 0 {
			return fmt.Errorf("node %s advertises nvidia.com/gpu; refusing to deploy the standalone NVIDIA DRA GPU plugin alongside a standard device plugin", node.Name)
		}
	}
	return nil
}

// WaitClusterPolicyReady waits for the GPU Operator to finish reconciling its
// driver, toolkit, and operands before a standalone DRA driver is started.
func WaitClusterPolicyReady(ctx context.Context, config *rest.Config) error {
	client, err := dynamic.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("creating NVIDIA API client: %w", err)
	}
	var last string
	err = wait.PollUntilContextTimeout(ctx, 5*time.Second, 12*time.Minute, true, func(ctx context.Context) (bool, error) {
		object, err := client.Resource(clusterPolicyGVR).Get(ctx, "cluster-policy", metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			last = "ClusterPolicy has not been created"
			return false, nil
		}
		if err != nil {
			return false, err
		}
		state, _, err := unstructured.NestedString(object.Object, "status", "state")
		if err != nil {
			return false, fmt.Errorf("reading ClusterPolicy status.state: %w", err)
		}
		last = fmt.Sprintf("status.state=%q", state)
		return state == "ready", nil
	})
	if err != nil {
		return fmt.Errorf("waiting for NVIDIA ClusterPolicy readiness (%s): %w", last, err)
	}
	return nil
}

// WorkloadPod exercises the operator's classic device plugin on a real GPU.
func WorkloadPod(namespace, name string) *corev1.Pod {
	limits := corev1.ResourceList{corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("1")}
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}, Spec: corev1.PodSpec{RestartPolicy: corev1.RestartPolicyNever,
		Containers: []corev1.Container{{Name: "test", Image: "nvcr.io/nvidia/cuda:12.8.1-base-ubuntu22.04", Command: []string{"/bin/sh", "-ec", "nvidia-smi -L && echo PASS"}, Resources: corev1.ResourceRequirements{Limits: limits}}}}}
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
