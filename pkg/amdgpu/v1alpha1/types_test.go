package v1alpha1

import (
	"encoding/json"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
)

func TestJSONFieldNamesMatchUpstream(t *testing.T) {
	dc := DeviceConfig{Spec: DeviceConfigSpec{
		Driver:       DriverSpec{Enable: ptr.To(false)},
		DevicePlugin: DevicePluginSpec{EnableDevicePlugin: ptr.To(false)},
		DRADriver:    DRADriverSpec{Enable: ptr.To(true), Image: "img", CmdLineArguments: map[string]string{"v": "4"}},
	}}

	b, err := json.Marshal(dc)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		`"driver":{"enable":false}`,
		`"devicePlugin":{"enableDevicePlugin":false}`,
		`"draDriver":{"enable":true,"image":"img","cmdLineArguments":{"v":"4"}}`,
	} {
		if !strings.Contains(string(b), want) {
			t.Errorf("JSON %s missing %s", b, want)
		}
	}
}

func TestSchemeRegistration(t *testing.T) {
	s := runtime.NewScheme()
	if err := AddToScheme(s); err != nil {
		t.Fatal(err)
	}

	gvks, _, err := s.ObjectKinds(&DeviceConfig{})
	if err != nil || len(gvks) != 1 || gvks[0].Kind != "DeviceConfig" || gvks[0].Group != "amd.com" {
		t.Fatalf("gvks=%v err=%v", gvks, err)
	}
}

func TestDeepCopyIsIndependent(t *testing.T) {
	orig := &DeviceConfig{Spec: DeviceConfigSpec{DRADriver: DRADriverSpec{CmdLineArguments: map[string]string{"v": "4"}}}}
	cp := orig.DeepCopy()
	cp.Spec.DRADriver.CmdLineArguments["v"] = "9"

	if orig.Spec.DRADriver.CmdLineArguments["v"] != "4" {
		t.Error("DeepCopy shares the map with the original")
	}
}
