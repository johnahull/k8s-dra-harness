// Package v1alpha1 is a trimmed copy of the AMD GPU Operator amd.com/v1alpha1
// API (github.com/ROCm/gpu-operator/api/v1alpha1). Only fields amd-gpu-e2e reads or
// sets are modeled; updates must go through merge patches (see pkg/amdgpu) so
// unmodeled fields on the server are preserved.
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DeviceConfig is the AMD GPU Operator's top-level custom resource.
type DeviceConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DeviceConfigSpec   `json:"spec,omitempty"`
	Status DeviceConfigStatus `json:"status,omitempty"`
}

// DeviceConfigList is a list of DeviceConfig.
type DeviceConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []DeviceConfig `json:"items"`
}

// DeviceConfigSpec is the subset of the upstream spec used by amd-gpu-e2e.
type DeviceConfigSpec struct {
	Driver       DriverSpec        `json:"driver,omitempty"`
	DevicePlugin DevicePluginSpec  `json:"devicePlugin,omitempty"`
	DRADriver    DRADriverSpec     `json:"draDriver,omitempty"`
	Selector     map[string]string `json:"selector,omitempty"`
}

// DriverSpec controls the amdgpu kernel module (built/loaded through KMM).
type DriverSpec struct {
	Enable  *bool  `json:"enable,omitempty"`
	Version string `json:"version,omitempty"`
	Image   string `json:"image,omitempty"`
}

// DevicePluginSpec controls the amd.com/gpu device plugin.
type DevicePluginSpec struct {
	EnableDevicePlugin *bool  `json:"enableDevicePlugin,omitempty"`
	DevicePluginImage  string `json:"devicePluginImage,omitempty"`
}

// DRADriverSpec controls the operator-managed DRA driver. The operator rejects
// enabling it together with the device plugin.
type DRADriverSpec struct {
	Enable           *bool             `json:"enable,omitempty"`
	Image            string            `json:"image,omitempty"`
	ImagePullPolicy  string            `json:"imagePullPolicy,omitempty"`
	CmdLineArguments map[string]string `json:"cmdLineArguments,omitempty"`
}

// DeviceConfigStatus is the subset of the upstream status used by amd-gpu-e2e.
type DeviceConfigStatus struct {
	DevicePlugin     DeploymentStatus        `json:"devicePlugin,omitempty"`
	Drivers          DeploymentStatus        `json:"driver,omitempty"`
	NodeModuleStatus map[string]ModuleStatus `json:"nodeModuleStatus,omitempty"`
	Conditions       []metav1.Condition      `json:"conditions,omitempty"`
}

// DeploymentStatus reports how many nodes run a component.
type DeploymentStatus struct {
	NodesMatchingSelectorNumber int32 `json:"nodesMatchingSelectorNumber,omitempty"`
	DesiredNumber               int32 `json:"desiredNumber,omitempty"`
	AvailableNumber             int32 `json:"availableNumber,omitempty"`
}

// ModuleStatus reports the kernel module state on one node.
type ModuleStatus struct {
	ContainerImage string `json:"containerImage,omitempty"`
	KernelVersion  string `json:"kernelVersion,omitempty"`
	Status         string `json:"status,omitempty"`
}
