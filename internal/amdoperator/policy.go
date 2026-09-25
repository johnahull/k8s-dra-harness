// Package amdoperator holds the AMD GPU Operator policy used by the harness.
package amdoperator

import (
	"github.com/johnahull/k8s-dra-harness/internal/driver"
	"github.com/johnahull/k8s-dra-harness/internal/runconfig"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// deviceConfigGVR identifies the operator's namespaced configuration resource.
var deviceConfigGVR = schema.GroupVersionResource{Group: "amd.com", Version: "v1alpha1", Resource: "deviceconfigs"}

// HasStandaloneDriver reports whether the run installs a separate AMD DRA
// driver, which must not compete with the operator-managed one.
func HasStandaloneDriver(drivers []runconfig.Driver) bool {
	for _, selected := range drivers {
		if selected.Name == "amd" {
			return true
		}
	}
	return false
}

// DeviceConfigSpec prepares a bundle installation's DeviceConfig without
// changing the supplied configuration.
func DeviceConfigSpec(spec map[string]any, standaloneAMD bool) map[string]any {
	out := cloneValues(spec)
	if standaloneAMD {
		setValue(out, false, "devicePlugin", "enableDevicePlugin")
		setValue(out, false, "draDriver", "enable")
	}
	return out
}

// ChartValues prepares the operator chart's values and image override.
func ChartValues(o *runconfig.AMDOperator, standaloneAMD bool) (map[string]any, error) {
	values := cloneValues(o.Values)
	if standaloneAMD {
		setValue(values, false, "deviceConfig", "spec", "devicePlugin", "enableDevicePlugin")
		setValue(values, false, "deviceConfig", "spec", "draDriver", "enable")
	}
	if o.Image != "" {
		repo, tag, err := driver.SplitImage(o.Image)
		if err != nil {
			return nil, err
		}
		setValue(values, map[string]any{"repository": repo, "tag": tag}, "controllerManager", "manager", "image")
	}
	return values, nil
}

// WorkloadPod exercises the operator's device plugin on a real GPU.
func WorkloadPod(namespace, name string) *corev1.Pod {
	limits := corev1.ResourceList{corev1.ResourceName("amd.com/gpu"): resource.MustParse("1")}
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}, Spec: corev1.PodSpec{RestartPolicy: corev1.RestartPolicyNever,
		Containers: []corev1.Container{{Name: "test", Image: "docker.io/rocm/dev-ubuntu-22.04:6.4", Command: []string{"/bin/sh", "-ec", "rocm-smi && echo PASS"}, Resources: corev1.ResourceRequirements{Limits: limits}}}}}
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
