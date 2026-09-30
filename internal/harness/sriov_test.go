package harness

import (
	"encoding/json"
	"testing"

	"github.com/johnahull/k8s-dra-harness/internal/runconfig"
	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestDriverNamespaceUsesSriovPolicyNamespace(t *testing.T) {
	runner := &Runner{Config: &runconfig.Config{Namespace: "dra-harness"}, ID: "test1234"}
	got := runner.driverNamespace(runconfig.Driver{Name: "sriov", SriovPolicy: &runconfig.SriovPolicy{Namespace: "sriov-driver"}})
	if got != "sriov-driver" {
		t.Fatalf("driver namespace = %q, want sriov-driver", got)
	}
}

func TestSriovPolicyObject(t *testing.T) {
	const namespace = "dra-sriov"
	object := sriovPolicyObject(namespace, "all-devices", "test1234", map[string]any{"configs": []any{map[string]any{}}})
	if got := object.GetAPIVersion(); got != "sriovnetwork.k8snetworkplumbingwg.io/v1alpha1" {
		t.Fatalf("apiVersion = %q", got)
	}
	if got := object.GetKind(); got != "SriovResourcePolicy" {
		t.Fatalf("kind = %q", got)
	}
	if got, found, err := unstructured.NestedFieldNoCopy(object.Object, "spec", "configs"); err != nil || !found {
		t.Fatalf("spec.configs = %#v, found=%t, err=%v", got, found, err)
	}
	if got := object.GetLabels()["dra-harness/run"]; got != "test1234" {
		t.Fatalf("run label = %q", got)
	}
	if !sriovPolicyOwned(object, "test1234") || sriovPolicyOwned(object, "another-run") {
		t.Fatal("unexpected SR-IOV policy ownership result")
	}
}

func TestConfigureDriverClaim(t *testing.T) {
	claim := &resourcev1.ResourceClaim{}
	if err := configureDriverClaim(claim, &runconfig.ClaimConfig{
		Requests: []string{"device"},
		Driver:   "sriovnetwork.k8snetworkplumbingwg.io",
		Parameters: map[string]any{
			"apiVersion":       "sriovnetwork.k8snetworkplumbingwg.io/v1alpha1",
			"kind":             "VfConfig",
			"netAttachDefName": "vf-test",
		},
	}); err != nil {
		t.Fatal(err)
	}
	if len(claim.Spec.Devices.Config) != 1 || claim.Spec.Devices.Config[0].Opaque == nil {
		t.Fatalf("claim config = %#v", claim.Spec.Devices.Config)
	}
	if got := claim.Spec.Devices.Config[0].Opaque.Driver; got != "sriovnetwork.k8snetworkplumbingwg.io" {
		t.Fatalf("claim driver = %q", got)
	}
	var parameters map[string]any
	if err := json.Unmarshal(claim.Spec.Devices.Config[0].Opaque.Parameters.Raw, &parameters); err != nil {
		t.Fatal(err)
	}
	if parameters["netAttachDefName"] != "vf-test" {
		t.Fatalf("claim parameters = %#v", parameters)
	}
}
