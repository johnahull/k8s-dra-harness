package v1alpha1

import (
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	// GroupVersion is the API group and version of DeviceConfig.
	GroupVersion = schema.GroupVersion{Group: "amd.com", Version: "v1alpha1"}

	// SchemeBuilder registers this package's types.
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)

	// AddToScheme adds this package's types to a scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)

func addKnownTypes(s *runtime.Scheme) error {
	s.AddKnownTypes(GroupVersion, &DeviceConfig{}, &DeviceConfigList{})
	metav1.AddToGroupVersion(s, GroupVersion)

	return nil
}

// DeepCopy returns an independent copy. A JSON round trip is used instead of
// generated deepcopy code; these objects are small and not on a hot path.
func (in *DeviceConfig) DeepCopy() *DeviceConfig {
	out := &DeviceConfig{}
	jsonCopy(in, out)

	return out
}

// DeepCopyObject implements runtime.Object.
func (in *DeviceConfig) DeepCopyObject() runtime.Object { return in.DeepCopy() }

// DeepCopyObject implements runtime.Object.
func (in *DeviceConfigList) DeepCopyObject() runtime.Object {
	out := &DeviceConfigList{}
	jsonCopy(in, out)

	return out
}

func jsonCopy(in, out any) {
	b, err := json.Marshal(in)
	if err != nil {
		panic(fmt.Sprintf("deepcopy marshal: %v", err))
	}

	if err := json.Unmarshal(b, out); err != nil {
		panic(fmt.Sprintf("deepcopy unmarshal: %v", err))
	}
}
