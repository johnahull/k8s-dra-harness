// Package amdgpu builds and manages the AMD GPU Operator DeviceConfig.
package amdgpu

import (
	"context"
	"fmt"

	amdv1alpha1 "github.com/johnahull/amd-gpu-e2e/pkg/amdgpu/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Builder wraps one DeviceConfig. Edit Definition through the With* methods,
// then Create (new object) or Apply (existing object).
type Builder struct {
	client     client.Client
	Definition *amdv1alpha1.DeviceConfig
	// original is the last state read from or written to the server; Apply
	// diffs against it so only changed fields are sent.
	original *amdv1alpha1.DeviceConfig
}

// NewBuilder starts a DeviceConfig that does not exist on the server yet.
func NewBuilder(c client.Client, name, namespace string) *Builder {
	return &Builder{
		client: c,
		Definition: &amdv1alpha1.DeviceConfig{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		},
	}
}

// Pull loads an existing DeviceConfig.
func Pull(ctx context.Context, c client.Client, name, namespace string) (*Builder, error) {
	dc := &amdv1alpha1.DeviceConfig{}
	if err := c.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, dc); err != nil {
		return nil, fmt.Errorf("getting DeviceConfig %s/%s: %w", namespace, name, err)
	}

	return &Builder{client: c, Definition: dc, original: dc.DeepCopy()}, nil
}

// WithSelector sets the node selector the operator uses to pick GPU nodes.
func (b *Builder) WithSelector(selector map[string]string) *Builder {
	b.Definition.Spec.Selector = selector

	return b
}

// WithDriver enables or disables operator-managed (KMM) driver install.
// Empty version or image leaves the current value unchanged.
func (b *Builder) WithDriver(enable bool, version, image string) *Builder {
	b.Definition.Spec.Driver.Enable = ptr.To(enable)

	if version != "" {
		b.Definition.Spec.Driver.Version = version
	}

	if image != "" {
		b.Definition.Spec.Driver.Image = image
	}

	return b
}

// WithDevicePlugin enables or disables the amd.com/gpu device plugin.
func (b *Builder) WithDevicePlugin(enable bool) *Builder {
	b.Definition.Spec.DevicePlugin.EnableDevicePlugin = ptr.To(enable)

	return b
}

// WithDRADriver enables or disables the operator-managed DRA driver. Empty
// image or args leave the current values unchanged.
func (b *Builder) WithDRADriver(enable bool, image string, args map[string]string) *Builder {
	b.Definition.Spec.DRADriver.Enable = ptr.To(enable)

	if image != "" {
		b.Definition.Spec.DRADriver.Image = image
	}

	if len(args) > 0 {
		b.Definition.Spec.DRADriver.CmdLineArguments = args
	}

	return b
}

// Exists reports whether the DeviceConfig is on the server.
func (b *Builder) Exists(ctx context.Context) (bool, error) {
	err := b.client.Get(ctx, client.ObjectKeyFromObject(b.Definition), &amdv1alpha1.DeviceConfig{})
	if apierrors.IsNotFound(err) {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("getting DeviceConfig %s: %w", b.key(), err)
	}

	return true, nil
}

// Create creates the DeviceConfig.
func (b *Builder) Create(ctx context.Context) error {
	if err := b.client.Create(ctx, b.Definition); err != nil {
		return fmt.Errorf("creating DeviceConfig %s: %w", b.key(), err)
	}

	b.original = b.Definition.DeepCopy()

	return nil
}

// Apply sends the fields changed since Pull/Create as a JSON merge patch, so
// spec fields this package does not model are left untouched on the server.
func (b *Builder) Apply(ctx context.Context) error {
	if b.original == nil {
		return fmt.Errorf("DeviceConfig %s: Apply requires Pull or Create first", b.key())
	}

	if err := b.client.Patch(ctx, b.Definition, client.MergeFrom(b.original)); err != nil {
		return fmt.Errorf("patching DeviceConfig %s: %w", b.key(), err)
	}

	b.original = b.Definition.DeepCopy()

	return nil
}

// Delete deletes the DeviceConfig; NotFound is not an error.
func (b *Builder) Delete(ctx context.Context) error {
	if err := b.client.Delete(ctx, b.Definition); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("deleting DeviceConfig %s: %w", b.key(), err)
	}

	return nil
}

func (b *Builder) key() string {
	return b.Definition.Namespace + "/" + b.Definition.Name
}
