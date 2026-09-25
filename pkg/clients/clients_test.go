package clients

import (
	"testing"

	amdv1alpha1 "github.com/johnahull/amd-ci/pkg/amdgpu/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestNewSchemeRegistersTypes(t *testing.T) {
	s, err := NewScheme()
	if err != nil {
		t.Fatal(err)
	}

	for _, obj := range []runtime.Object{&corev1.Node{}, &resourcev1.ResourceSlice{}, &amdv1alpha1.DeviceConfig{}} {
		if _, _, err := s.ObjectKinds(obj); err != nil {
			t.Errorf("%T not registered: %v", obj, err)
		}
	}
}
