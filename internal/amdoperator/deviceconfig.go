package amdoperator

import (
	"context"
	"errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

// CheckDeviceConfigs refuses to deploy another operator when a DeviceConfig
// already exists anywhere on the cluster.
func CheckDeviceConfigs(ctx context.Context, config *rest.Config) error {
	client, err := dynamic.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("creating AMD API client: %w", err)
	}
	configs, err := client.Resource(deviceConfigGVR).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("checking existing DeviceConfigs: %w", err)
	}
	if err == nil && len(configs.Items) > 0 {
		return errors.New("AMD DeviceConfigs already exist; refusing to install another operator")
	}
	return nil
}

// CreateDeviceConfig creates a configuration owned by the current run.
func CreateDeviceConfig(ctx context.Context, config *rest.Config, namespace, name string, spec map[string]any) error {
	client, err := dynamic.NewForConfig(config)
	if err != nil {
		return err
	}
	obj := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "amd.com/v1alpha1", "kind": "DeviceConfig", "metadata": map[string]any{"name": name, "namespace": namespace}, "spec": spec}}
	if _, err := client.Resource(deviceConfigGVR).Namespace(namespace).Create(ctx, obj, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("creating DeviceConfig: %w", err)
	}
	return nil
}

// DeleteDeviceConfig removes a configuration owned by the current run.
func DeleteDeviceConfig(ctx context.Context, config *rest.Config, namespace, name string) error {
	client, err := dynamic.NewForConfig(config)
	if err != nil {
		return err
	}
	if err := client.Resource(deviceConfigGVR).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("deleting DeviceConfig %s: %w", name, err)
	}
	return nil
}
