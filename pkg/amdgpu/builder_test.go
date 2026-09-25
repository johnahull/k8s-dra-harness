package amdgpu

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	amdv1alpha1 "github.com/johnahull/k8s-dra-harness/pkg/amdgpu/v1alpha1"
	"github.com/johnahull/k8s-dra-harness/pkg/clients"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	testName = "test-deviceconfig"
	testNs   = "kube-amd-gpu"
)

func newFakeClient(t *testing.T) client.Client {
	t.Helper()

	s, err := clients.NewScheme(amdv1alpha1.AddToScheme)
	if err != nil {
		t.Fatal(err)
	}

	return fake.NewClientBuilder().WithScheme(s).Build()
}

func TestCreateThenPull(t *testing.T) {
	ctx := context.Background()
	c := newFakeClient(t)

	err := NewBuilder(c, testName, testNs).
		WithSelector(map[string]string{"feature.node.kubernetes.io/amd-gpu": "true"}).
		WithDriver(false, "", "").
		WithDevicePlugin(true).
		Create(ctx)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	b, err := Pull(ctx, c, testName, testNs)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}

	spec := b.Definition.Spec
	if *spec.Driver.Enable || !*spec.DevicePlugin.EnableDevicePlugin || spec.Selector["feature.node.kubernetes.io/amd-gpu"] != "true" {
		t.Errorf("unexpected spec: %+v", spec)
	}
}

func TestApplySendsOnlyChangedFields(t *testing.T) {
	ctx := context.Background()
	c := newFakeClient(t)

	if err := NewBuilder(c, testName, testNs).WithDriver(true, "6.4.1", "").WithDevicePlugin(true).Create(ctx); err != nil {
		t.Fatal(err)
	}

	b, err := Pull(ctx, c, testName, testNs)
	if err != nil {
		t.Fatal(err)
	}

	b.WithDevicePlugin(false).WithDRADriver(true, "quay.io/x/dra:dev", map[string]string{"v": "4"})

	raw, err := client.MergeFrom(b.original).Data(b.Definition)
	if err != nil {
		t.Fatal(err)
	}

	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}

	want := map[string]any{"spec": map[string]any{
		"devicePlugin": map[string]any{"enableDevicePlugin": false},
		"draDriver":    map[string]any{"enable": true, "image": "quay.io/x/dra:dev", "cmdLineArguments": map[string]any{"v": "4"}},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("patch = %s, want only devicePlugin+draDriver changes", raw)
	}

	if err := b.Apply(ctx); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	after, err := Pull(ctx, c, testName, testNs)
	if err != nil {
		t.Fatal(err)
	}

	if *after.Definition.Spec.DevicePlugin.EnableDevicePlugin || !*after.Definition.Spec.DRADriver.Enable ||
		after.Definition.Spec.Driver.Version != "6.4.1" {
		t.Errorf("unexpected spec after Apply: %+v", after.Definition.Spec)
	}
}

func TestApplyRequiresPullOrCreate(t *testing.T) {
	if err := NewBuilder(newFakeClient(t), testName, testNs).Apply(context.Background()); err == nil {
		t.Fatal("expected error applying a builder that was never created or pulled")
	}
}

func TestExistsAndDelete(t *testing.T) {
	ctx := context.Background()
	c := newFakeClient(t)
	b := NewBuilder(c, testName, testNs)

	if ok, err := b.Exists(ctx); ok || err != nil {
		t.Fatalf("Exists before create = %v, %v", ok, err)
	}

	if err := b.Create(ctx); err != nil {
		t.Fatal(err)
	}

	if ok, err := b.Exists(ctx); !ok || err != nil {
		t.Fatalf("Exists after create = %v, %v", ok, err)
	}

	if err := b.Delete(ctx); err != nil {
		t.Fatal(err)
	}

	if err := b.Delete(ctx); err != nil {
		t.Fatalf("second Delete should ignore NotFound: %v", err)
	}
}
