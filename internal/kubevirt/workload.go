// Package kubevirt contains the optional direct-VMI workload backend.
package kubevirt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/johnahull/k8s-dra-harness/internal/driver"
	"github.com/johnahull/k8s-dra-harness/internal/runconfig"
	"github.com/johnahull/k8s-dra-harness/pkg/clients"
	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	virtv1 "kubevirt.io/api/core/v1"
)

const (
	requestName    = "device"
	cpuRequestName = "cpu"
	checkTimeout   = 10 * time.Minute
	launcherLabel  = "vmi.kubevirt.io/id"
)

// ErrUnsupported identifies a cluster that cannot run the selected KubeVirt
// workload. Integration suites may skip such a compatibility target while
// preserving configuration and workload failures as test failures.
var ErrUnsupported = errors.New("KubeVirt DRA workload is unsupported")

// Runner executes direct-VMI DRA workloads using the cluster clients owned by
// the harness. It never installs or changes KubeVirt configuration.
type Runner struct {
	clients *clients.Settings
}

// New creates a KubeVirt workload runner.
func New(settings *clients.Settings) *Runner { return &Runner{clients: settings} }

// Preflight verifies the APIs and KubeVirt feature gate needed by the
// selected attachment. Feature gates are observed from the installed KubeVirt
// custom resource; the harness never enables them.
func (r *Runner) Preflight(ctx context.Context, attachment string) error {
	resources, err := r.clients.Discovery.ServerResourcesForGroupVersion("kubevirt.io/v1")
	if err != nil {
		return fmt.Errorf("%w: cluster must serve kubevirt.io/v1: %v", ErrUnsupported, err)
	}
	foundVMI := false
	for _, resource := range resources.APIResources {
		if resource.Name == "virtualmachineinstances" && resource.Namespaced {
			foundVMI = true
			break
		}
	}
	if !foundVMI {
		return fmt.Errorf("%w: kubevirt.io/v1 does not expose namespaced virtualmachineinstances", ErrUnsupported)
	}

	gate, defaultEnabled := kubeVirtFeatureGate(attachment)
	if gate == "" {
		return fmt.Errorf("%w: unsupported KubeVirt attachment %q", ErrUnsupported, attachment)
	}
	list, err := r.clients.Dynamic.Resource(schema.GroupVersionResource{
		Group: "kubevirt.io", Version: "v1", Resource: "kubevirts",
	}).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("%w: discovering KubeVirt configuration: %v", ErrUnsupported, err)
	}
	if len(list.Items) == 0 {
		return fmt.Errorf("%w: no KubeVirt custom resource found", ErrUnsupported)
	}
	for _, item := range list.Items {
		gates, found, err := nestedStringSlice(item.Object, "spec", "configuration", "developerConfiguration", "featureGates")
		if err != nil {
			return fmt.Errorf("%w: reading feature gates from KubeVirt %s/%s: %v", ErrUnsupported, item.GetNamespace(), item.GetName(), err)
		}
		if found && contains(gates, gate) {
			return nil
		}
		disabled, found, err := nestedStringSlice(item.Object, "spec", "configuration", "developerConfiguration", "disabledFeatureGates")
		if err != nil {
			return fmt.Errorf("%w: reading disabled feature gates from KubeVirt %s/%s: %v", ErrUnsupported, item.GetNamespace(), item.GetName(), err)
		}
		if found && contains(disabled, gate) {
			return fmt.Errorf("%w: KubeVirt feature gate %q is explicitly disabled", ErrUnsupported, gate)
		}
		version, found, err := nestedString(item.Object, "status", "observedKubeVirtVersion")
		if err != nil {
			return fmt.Errorf("%w: reading KubeVirt version from %s/%s: %v", ErrUnsupported, item.GetNamespace(), item.GetName(), err)
		}
		if defaultEnabled && found && kubeVirtVersionAtLeast(version, 1, 9) {
			// GPUsWithDRA and HostDevicesWithDRA are enabled by default in
			// KubeVirt v1.9 and later. An empty featureGates list therefore
			// does not mean that those DRA attachments are unavailable.
			return nil
		}
	}
	return fmt.Errorf("%w: KubeVirt is installed but feature gate %q is not enabled", ErrUnsupported, gate)
}

// Run creates one direct VMI and one user-owned ResourceClaim, then verifies
// allocation, virt-launcher propagation, VMI startup, and optional guest use.
func (r *Runner) Run(ctx context.Context, namespace, id string, config *runconfig.KubeVirt, selected driver.Adapter, driverName, deviceClass string) error {
	device, ok := driver.KubeVirtDeviceFor(selected)
	if !ok {
		return fmt.Errorf("driver %q has no KubeVirt device mapping", driverName)
	}
	attachment := config.Attachment
	if attachment == "" {
		attachment = device.Attachment
	}
	if attachment != runconfig.KubeVirtAttachmentGPU && attachment != runconfig.KubeVirtAttachmentHostDevice && attachment != runconfig.KubeVirtAttachmentCPU && attachment != runconfig.KubeVirtAttachmentNetwork {
		return fmt.Errorf("unsupported KubeVirt attachment %q", attachment)
	}
	if err := validateDeviceCount(config, attachment); err != nil {
		return err
	}
	guestDeviceName := config.DeviceName
	if guestDeviceName == "" {
		guestDeviceName = device.Name
	}
	if guestDeviceName == "" {
		guestDeviceName = driverName + "-device"
	}

	claimName := fmt.Sprintf("dra-vmi-%s-%s-claim", id, driverName)
	vmiName := fmt.Sprintf("dra-vmi-%s-%s", id, driverName)
	labels := map[string]string{
		"app.kubernetes.io/managed-by": "k8s-dra-harness",
		"app.kubernetes.io/component":  "kubevirt-workload",
		"dra-harness.io/run":           id,
	}
	claim, err := BuildClaim(claimName, namespace, config, driverName, deviceClass, labels)
	if err != nil {
		return err
	}
	if _, err := r.clients.K8s.ResourceV1().ResourceClaims(namespace).Create(ctx, claim, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("creating KubeVirt %s claim: %w", driverName, err)
	}
	createdVMI := false
	cleanup := func() error {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		var problems []error
		claimSafeToDelete := !createdVMI
		if createdVMI {
			if err := r.clients.Kubevirt.KubevirtV1().VirtualMachineInstances(namespace).Delete(cleanupCtx, vmiName, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
				problems = append(problems, fmt.Errorf("deleting VMI %s: %w", vmiName, err))
			}
			launcherGone := r.waitForLauncherGone(cleanupCtx, namespace, vmiName)
			if launcherGone != nil {
				problems = append(problems, launcherGone)
			}
			vmiGone := waitForNotFound(cleanupCtx, func(ctx context.Context) error {
				_, err := r.clients.Kubevirt.KubevirtV1().VirtualMachineInstances(namespace).Get(ctx, vmiName, metav1.GetOptions{})
				return err
			})
			if vmiGone != nil {
				problems = append(problems, fmt.Errorf("waiting for VMI %s deletion: %w", vmiName, vmiGone))
			}
			claimSafeToDelete = launcherGone == nil && vmiGone == nil
		}
		if !claimSafeToDelete {
			problems = append(problems, fmt.Errorf("retaining ResourceClaim %s because the VMI cleanup was not confirmed", claimName))
			return errors.Join(problems...)
		}
		if err := r.clients.K8s.ResourceV1().ResourceClaims(namespace).Delete(cleanupCtx, claimName, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			problems = append(problems, fmt.Errorf("deleting ResourceClaim %s: %w", claimName, err))
		}
		if err := waitForNotFound(cleanupCtx, func(ctx context.Context) error {
			_, err := r.clients.K8s.ResourceV1().ResourceClaims(namespace).Get(ctx, claimName, metav1.GetOptions{})
			return err
		}); err != nil {
			problems = append(problems, fmt.Errorf("waiting for ResourceClaim %s deletion: %w", claimName, err))
		}
		return errors.Join(problems...)
	}

	vmi, err := BuildVMI(vmiName, namespace, config, attachment, guestDeviceName, driverName, claimName, labels)
	if err != nil {
		return errors.Join(err, cleanup())
	}
	if _, err := r.clients.Kubevirt.KubevirtV1().VirtualMachineInstances(namespace).Create(ctx, vmi, metav1.CreateOptions{}); err != nil {
		return errors.Join(fmt.Errorf("creating KubeVirt VMI: %w", err), cleanup())
	}
	createdVMI = true
	if err := r.waitReady(ctx, namespace, vmiName, claimName); err != nil {
		return errors.Join(err, cleanup())
	}
	if config.Guest != nil {
		if err := verifyGuest(ctx, r.clients, namespace, vmiName, config.Guest); err != nil {
			return errors.Join(err, cleanup())
		}
	}
	if config.HoldAfterReadySeconds > 0 {
		timer := time.NewTimer(time.Duration(config.HoldAfterReadySeconds) * time.Second)
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return errors.Join(ctx.Err(), cleanup())
		}
	}
	return cleanup()
}

// BuildClaim returns the ResourceClaim consumed by a KubeVirt VMI. The
// selector, capacity, and opaque configuration are optional so the same
// backend can test ordinary GPU claims, grouped CPU claims, and driver-specific
// VFIO/SR-IOV claims.
func BuildClaim(name, namespace string, config *runconfig.KubeVirt, driverName, deviceClass string, labels map[string]string) (*resourcev1.ResourceClaim, error) {
	count, err := kubeVirtDeviceCount(config)
	if err != nil {
		return nil, err
	}
	if count > 1 && (driverName == "cpu" || deviceClass == "dra.cpu") {
		return nil, fmt.Errorf("kubevirt deviceCount greater than one is not supported for CPU claims")
	}
	exact := &resourcev1.ExactDeviceRequest{DeviceClassName: deviceClass}
	if config != nil && config.Selector != "" {
		exact.Selectors = []resourcev1.DeviceSelector{{CEL: &resourcev1.CELDeviceSelector{Expression: config.Selector}}}
	}
	if capacity, err := kubeVirtClaimCapacity(config, driverName, deviceClass); err != nil {
		return nil, err
	} else if len(capacity) > 0 {
		exact.Capacity = &resourcev1.CapacityRequirements{Requests: capacity}
	}
	requests := make([]resourcev1.DeviceRequest, count)
	for i, name := range kubeVirtRequestNames(driverName, deviceClass, count) {
		requests[i] = resourcev1.DeviceRequest{Name: name, Exactly: exact}
	}
	claim := &resourcev1.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: labels},
		Spec:       resourcev1.ResourceClaimSpec{Devices: resourcev1.DeviceClaim{Requests: requests}},
	}
	if config == nil || config.ClaimConfig == nil || (config.ClaimConfig.Driver == "" && len(config.ClaimConfig.Parameters) == 0) {
		return claim, nil
	}
	parameters, err := json.Marshal(config.ClaimConfig.Parameters)
	if err != nil {
		return nil, fmt.Errorf("encoding KubeVirt claim parameters: %w", err)
	}
	claim.Spec.Devices.Config = []resourcev1.DeviceClaimConfiguration{{
		Requests: config.ClaimConfig.Requests,
		DeviceConfiguration: resourcev1.DeviceConfiguration{Opaque: &resourcev1.OpaqueDeviceConfiguration{
			Driver:     config.ClaimConfig.Driver,
			Parameters: runtime.RawExtension{Raw: parameters},
		}},
	}}
	return claim, nil
}

// BuildVMI returns a direct-VMI object with the DRA claim wired into the
// selected KubeVirt GPU, HostDevice, CPU, or SR-IOV network field.
func BuildVMI(name, namespace string, config *runconfig.KubeVirt, attachment, deviceName, claimEntry, claimName string, labels map[string]string) (*virtv1.VirtualMachineInstance, error) {
	if err := validateDeviceCount(config, attachment); err != nil {
		return nil, err
	}
	count, err := kubeVirtDeviceCount(config)
	if err != nil {
		return nil, err
	}
	devices := virtv1.Devices{
		Disks: []virtv1.Disk{{Name: "rootdisk", DiskDevice: virtv1.DiskDevice{Disk: &virtv1.DiskTarget{Bus: virtv1.DiskBusVirtio}}}},
	}
	var networks []virtv1.Network
	var annotations map[string]string
	var cpu *virtv1.CPU
	switch attachment {
	case runconfig.KubeVirtAttachmentHostDevice:
		devices.HostDevices = kubeVirtHostDevices(deviceName, claimEntry, kubeVirtRequestNamesForAttachment(attachment, count))
	case runconfig.KubeVirtAttachmentGPU:
		devices.GPUs = kubeVirtGPUs(deviceName, claimEntry, kubeVirtRequestNamesForAttachment(attachment, count))
	case runconfig.KubeVirtAttachmentCPU:
		cpu = &virtv1.CPU{Cores: 1, DedicatedCPUPlacement: true}
		annotations = map[string]string{"kubevirt.io/dra-manual-claim": claimEntry}
	case runconfig.KubeVirtAttachmentNetwork:
		claimRef := &virtv1.ClaimRequest{ClaimName: claimEntry, RequestName: kubeVirtRequestNamesForAttachment(attachment, count)[0]}
		networks = []virtv1.Network{
			{Name: "default", NetworkSource: virtv1.NetworkSource{Pod: &virtv1.PodNetwork{}}},
			{Name: deviceName, NetworkSource: virtv1.NetworkSource{ResourceClaim: claimRef}},
		}
		devices.Interfaces = []virtv1.Interface{
			{Name: "default", InterfaceBindingMethod: virtv1.InterfaceBindingMethod{Masquerade: &virtv1.InterfaceMasquerade{}}},
			{Name: deviceName, InterfaceBindingMethod: virtv1.InterfaceBindingMethod{SRIOV: &virtv1.InterfaceSRIOV{}}},
		}
	}
	volumes := []virtv1.Volume{{Name: "rootdisk", VolumeSource: virtv1.VolumeSource{ContainerDisk: &virtv1.ContainerDiskSource{Image: config.Image}}}}
	if config.CloudInitSecret != "" {
		volumes = append(volumes, virtv1.Volume{Name: "cloudinit", VolumeSource: virtv1.VolumeSource{CloudInitNoCloud: &virtv1.CloudInitNoCloudSource{
			UserDataSecretRef: &corev1.LocalObjectReference{Name: config.CloudInitSecret},
		}}})
		readOnly := true
		devices.Disks = append(devices.Disks, virtv1.Disk{Name: "cloudinit", DiskDevice: virtv1.DiskDevice{CDRom: &virtv1.CDRomTarget{Bus: virtv1.DiskBusVirtio, ReadOnly: &readOnly}}})
	}
	return &virtv1.VirtualMachineInstance{
		TypeMeta:   metav1.TypeMeta{APIVersion: "kubevirt.io/v1", Kind: "VirtualMachineInstance"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: labels, Annotations: annotations},
		Spec: virtv1.VirtualMachineInstanceSpec{
			Domain: virtv1.DomainSpec{
				Resources: virtv1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceMemory: resourceQuantity("1Gi")}},
				CPU:       cpu,
				Devices:   devices,
			},
			Volumes:        volumes,
			Networks:       networks,
			ResourceClaims: []virtv1.VirtualMachineInstanceResourceClaim{{Name: claimEntry, ResourceClaimName: stringPtr(claimName)}},
		},
	}, nil
}

func kubeVirtFeatureGate(attachment string) (string, bool) {
	switch attachment {
	case runconfig.KubeVirtAttachmentGPU:
		return "GPUsWithDRA", true
	case runconfig.KubeVirtAttachmentHostDevice:
		return "HostDevicesWithDRA", true
	case runconfig.KubeVirtAttachmentCPU:
		return "CPUsWithDRA", false
	case runconfig.KubeVirtAttachmentNetwork:
		return "NetworkDevicesWithDRA", false
	default:
		return "", false
	}
}

func kubeVirtRequestNames(driverName, deviceClass string, count int) []string {
	if driverName == "cpu" || deviceClass == "dra.cpu" {
		return []string{cpuRequestName}
	}
	return kubeVirtRequestNamesForAttachment(runconfig.KubeVirtAttachmentGPU, count)
}

func kubeVirtRequestNamesForAttachment(attachment string, count int) []string {
	if attachment == runconfig.KubeVirtAttachmentCPU {
		return []string{cpuRequestName}
	}
	if count == 1 {
		return []string{"device"}
	}
	requests := make([]string, count)
	for i := range requests {
		requests[i] = fmt.Sprintf("device-%d", i)
	}
	return requests
}

func kubeVirtDeviceCount(config *runconfig.KubeVirt) (int, error) {
	count := 1
	if config != nil && config.DeviceCount != 0 {
		count = config.DeviceCount
	}
	if count < 1 {
		return 0, fmt.Errorf("kubevirt deviceCount must be greater than zero")
	}
	return count, nil
}

func validateDeviceCount(config *runconfig.KubeVirt, attachment string) error {
	count, err := kubeVirtDeviceCount(config)
	if err != nil {
		return err
	}
	if count > 1 && attachment != runconfig.KubeVirtAttachmentGPU && attachment != runconfig.KubeVirtAttachmentHostDevice {
		return fmt.Errorf("kubevirt deviceCount greater than one is only supported for gpu or hostDevice attachments")
	}
	return nil
}

func kubeVirtDeviceNames(base string, count int) []string {
	if count == 1 {
		return []string{base}
	}
	names := make([]string, count)
	for i := range names {
		names[i] = fmt.Sprintf("%s-%d", base, i)
	}
	return names
}

func kubeVirtGPUs(base, claimEntry string, requestNames []string) []virtv1.GPU {
	devices := make([]virtv1.GPU, len(requestNames))
	for i, name := range kubeVirtDeviceNames(base, len(requestNames)) {
		devices[i] = virtv1.GPU{Name: name, ClaimRequest: &virtv1.ClaimRequest{ClaimName: claimEntry, RequestName: requestNames[i]}}
	}
	return devices
}

func kubeVirtHostDevices(base, claimEntry string, requestNames []string) []virtv1.HostDevice {
	devices := make([]virtv1.HostDevice, len(requestNames))
	for i, name := range kubeVirtDeviceNames(base, len(requestNames)) {
		devices[i] = virtv1.HostDevice{Name: name, ClaimRequest: &virtv1.ClaimRequest{ClaimName: claimEntry, RequestName: requestNames[i]}}
	}
	return devices
}

func kubeVirtClaimCapacity(config *runconfig.KubeVirt, driverName, deviceClass string) (map[resourcev1.QualifiedName]resource.Quantity, error) {
	capacity := map[resourcev1.QualifiedName]resource.Quantity{}
	if config != nil && config.ClaimConfig != nil {
		for name, value := range config.ClaimConfig.Capacity {
			quantity, err := resource.ParseQuantity(fmt.Sprint(value))
			if err != nil {
				return nil, fmt.Errorf("parsing KubeVirt claim capacity %q: %w", name, err)
			}
			if quantity.Sign() <= 0 {
				return nil, fmt.Errorf("KubeVirt claim capacity %q must be greater than zero", name)
			}
			capacity[resourcev1.QualifiedName(name)] = quantity
		}
	}
	if driverName == "cpu" || deviceClass == "dra.cpu" {
		if len(capacity) == 0 {
			capacity[resourcev1.QualifiedName("dra.cpu/cpu")] = resource.MustParse("1")
		}
	}
	return capacity, nil
}

func (r *Runner) waitReady(ctx context.Context, namespace, vmiName, claimName string) error {
	var last string
	err := wait.PollUntilContextTimeout(ctx, 5*time.Second, checkTimeout, true, func(ctx context.Context) (bool, error) {
		claim, err := r.clients.K8s.ResourceV1().ResourceClaims(namespace).Get(ctx, claimName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if claim.Status.Allocation == nil {
			last = "ResourceClaim is not allocated"
			return false, nil
		}
		vmi, err := r.clients.Kubevirt.KubevirtV1().VirtualMachineInstances(namespace).Get(ctx, vmiName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		last = fmt.Sprintf("VMI phase=%s reason=%s", vmi.Status.Phase, vmi.Status.Reason)
		if vmi.Status.Phase == virtv1.Failed {
			return false, errors.New(last)
		}
		pods, err := r.clients.K8s.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: launcherLabel + "=" + vmiName})
		if err != nil {
			return false, err
		}
		reserved := false
		for _, pod := range pods.Items {
			if !claimReservedForPod(claim, &pod) {
				continue
			}
			reserved = true
			if vmi.Status.Phase == virtv1.Running {
				return true, nil
			}
			last += "; virt-launcher claim reserved"
		}
		if !reserved {
			last += "; virt-launcher claim is not reserved for the pod"
		}
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("waiting for KubeVirt workload (%s): %w", last, err)
	}
	return nil
}

func (r *Runner) waitForLauncherGone(ctx context.Context, namespace, vmiName string) error {
	err := wait.PollUntilContextTimeout(ctx, 2*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		pods, err := r.clients.K8s.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: launcherLabel + "=" + vmiName})
		if err != nil {
			return false, err
		}
		return len(pods.Items) == 0, nil
	})
	if err != nil {
		return fmt.Errorf("waiting for virt-launcher pod for VMI %s deletion: %w", vmiName, err)
	}
	return nil
}

func podCarriesClaim(pod *corev1.Pod, claimName string) bool {
	for _, claim := range pod.Spec.ResourceClaims {
		if claim.ResourceClaimName != nil && *claim.ResourceClaimName == claimName {
			return true
		}
	}
	return false
}

func claimReservedForPod(claim *resourcev1.ResourceClaim, pod *corev1.Pod) bool {
	for _, consumer := range claim.Status.ReservedFor {
		if consumer.Resource == "pods" && consumer.Name == pod.Name && consumer.UID == pod.UID {
			return true
		}
	}
	return false
}

func waitForNotFound(ctx context.Context, get func(context.Context) error) error {
	return wait.PollUntilContextTimeout(ctx, 2*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		err := get(ctx)
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		return false, nil
	})
}

func nestedStringSlice(obj map[string]any, fields ...string) ([]string, bool, error) {
	value, found, err := unstructuredNestedFieldNoCopy(obj, fields...)
	if err != nil || !found {
		return nil, found, err
	}
	values, ok := value.([]any)
	if !ok {
		return nil, true, fmt.Errorf("expected string list, got %T", value)
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		stringValue, ok := value.(string)
		if !ok {
			return nil, true, fmt.Errorf("expected string feature gate, got %T", value)
		}
		out = append(out, stringValue)
	}
	return out, true, nil
}

func nestedString(obj map[string]any, fields ...string) (string, bool, error) {
	value, found, err := unstructuredNestedFieldNoCopy(obj, fields...)
	if err != nil || !found {
		return "", found, err
	}
	stringValue, ok := value.(string)
	if !ok {
		return "", true, fmt.Errorf("expected string, got %T", value)
	}
	return stringValue, true, nil
}

func kubeVirtVersionAtLeast(version string, wantMajor, wantMinor int) bool {
	version = strings.TrimPrefix(version, "v")
	parts := strings.SplitN(version, ".", 3)
	if len(parts) < 2 {
		return false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return false
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return false
	}
	return major > wantMajor || (major == wantMajor && minor >= wantMinor)
}

func unstructuredNestedFieldNoCopy(obj map[string]any, fields ...string) (any, bool, error) {
	var current any = obj
	for _, field := range fields {
		mapping, ok := current.(map[string]any)
		if !ok {
			return nil, false, nil
		}
		value, ok := mapping[field]
		if !ok {
			return nil, false, nil
		}
		current = value
	}
	return current, true, nil
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func stringPtr(value string) *string { return &value }

func resourceQuantity(value string) resource.Quantity { return resource.MustParse(value) }
