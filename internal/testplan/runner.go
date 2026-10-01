// Package testplan runs namespace-scoped DRA allocation scenarios.
package testplan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/johnahull/k8s-dra-harness/internal/driver"
	"github.com/johnahull/k8s-dra-harness/internal/runconfig"
	"github.com/johnahull/k8s-dra-harness/internal/verification"
	"github.com/johnahull/k8s-dra-harness/pkg/clients"
	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	defaultWorkloadImage = "registry.k8s.io/e2e-test-images/busybox:1.29-4"
	scenarioTimeout      = 3 * time.Minute
	pendingTimeout       = 30 * time.Second
)

// DriverTarget identifies one installed or pre-existing DRA driver for a
// restart scenario.
type DriverTarget struct {
	Name      string
	Namespace string
	Selector  string
}

// Runner owns only objects carrying its owner label. It does not delete
// cluster-scoped DRA objects or resources belonging to an installed driver.
type Runner struct {
	clients   *clients.Settings
	config    *runconfig.TestPlan
	namespace string
	owner     string
	verify    *verification.Runner
	targets   []DriverTarget
	claims    []string
	pods      []string
	templates []string
}

// New creates a scenario runner. The caller is responsible for ensuring the
// namespace exists before invoking Run.
func New(client *clients.Settings, config *runconfig.TestPlan, namespace, kubeconfig string) (*Runner, error) {
	if client == nil {
		return nil, errors.New("test plan requires Kubernetes clients")
	}
	if config == nil {
		return nil, errors.New("test plan configuration is nil")
	}
	verify, err := verification.New(verification.Config{
		ScriptsDir:         config.Verification.ScriptsDir,
		ExpectedRepoCommit: config.Verification.ExpectedRepoCommit,
		EvidenceDir:        config.Verification.EvidenceDir,
		Commands:           config.Verification.Commands,
	}, kubeconfig)
	if err != nil {
		return nil, err
	}
	return &Runner{
		clients:   client,
		config:    config,
		namespace: namespace,
		owner:     "testplan",
		verify:    verify,
	}, nil
}

// Run executes configured scenarios in the declared order and always cleans
// up claims, templates, and consumers created by this runner.
func (r *Runner) Run(ctx context.Context, targets []DriverTarget) (runErr error) {
	r.targets = append([]DriverTarget(nil), targets...)
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := r.Cleanup(cleanupCtx); err != nil {
			runErr = errors.Join(runErr, err)
		}
	}()
	for _, scenario := range r.config.Scenarios {
		var err error
		switch scenario {
		case "resource-slices":
			err = r.resourceSlices(ctx)
		case "counters":
			err = r.counters(ctx)
		case "sibling-exclusion":
			err = r.siblingExclusion(ctx)
		case "sibling-exclusion-reverse":
			err = r.siblingExclusionOrder(ctx, true)
		case "alternate-device":
			err = r.alternateDevice(ctx)
		case "capacity":
			err = r.capacity(ctx)
		case "release":
			err = r.release(ctx)
		case "release-orders":
			err = r.releaseOrders(ctx)
		case "topology":
			err = r.topology(ctx)
		case "restart":
			if !r.config.Lifecycle.AllowRestart {
				err = errors.New("test plan restart requires lifecycle.allowRestart: true")
			} else {
				err = r.restart(ctx, targets)
			}
		case "restart-active":
			if !r.config.Lifecycle.AllowRestart {
				err = errors.New("test plan restart-active requires lifecycle.allowRestart: true")
			} else {
				err = r.restartActive(ctx, targets)
			}
		default:
			err = fmt.Errorf("unsupported test plan scenario %q", scenario)
		}
		if err != nil {
			return fmt.Errorf("scenario %s: %w", scenario, err)
		}
		switch scenario {
		case "sibling-exclusion", "sibling-exclusion-reverse", "alternate-device", "capacity", "release", "release-orders":
			if _, err := r.snapshot(ctx, scenario); err != nil {
				return fmt.Errorf("capturing %s evidence: %w", scenario, err)
			}
			if err := r.runDefaultVerifiers(ctx, scenario); err != nil {
				return fmt.Errorf("verifying %s: %w", scenario, err)
			}
		}
		if err := r.Cleanup(ctx); err != nil {
			return fmt.Errorf("cleaning scenario %s: %w", scenario, err)
		}
	}
	return nil
}

func (r *Runner) resourceSlices(ctx context.Context) error {
	if err := r.verifyResources(ctx); err != nil {
		return err
	}
	if _, err := r.snapshot(ctx, "resource-slices"); err != nil {
		return err
	}
	return r.runDefaultVerifiers(ctx, "resource-slices")
}

func (r *Runner) counters(ctx context.Context) error {
	slices, err := r.clients.K8s.ResourceV1().ResourceSlices().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing ResourceSlices: %w", err)
	}
	found := false
	for _, slice := range slices.Items {
		if len(slice.Spec.SharedCounters) > 0 {
			found = true
			break
		}
	}
	if !found {
		return errors.New("no shared counters were published")
	}
	snapshot, err := r.snapshot(ctx, "counters")
	if err != nil {
		return err
	}
	if err := r.verify.RunCounterReport(ctx, r.namespace, snapshot); err != nil {
		return err
	}
	return r.runVerifier(ctx, "counters", "dra-verify.sh", "counters")
}

func (r *Runner) verifyResources(ctx context.Context) error {
	slices, err := r.clients.K8s.ResourceV1().ResourceSlices().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing ResourceSlices: %w", err)
	}
	if len(slices.Items) == 0 {
		return errors.New("no ResourceSlices were published")
	}
	return nil
}

func (r *Runner) topology(ctx context.Context) error {
	if len(r.config.Topology) == 0 {
		return errors.New("topology scenario requires testPlan.topology cases")
	}
	for _, topology := range r.config.Topology {
		if err := r.runTopologyCase(ctx, topology); err != nil {
			return err
		}
		prefix := "topology-" + topology.Name
		if _, err := r.snapshot(ctx, prefix); err != nil {
			return fmt.Errorf("capturing topology case %s evidence: %w", topology.Name, err)
		}
		if err := r.runDefaultVerifiers(ctx, prefix); err != nil {
			return fmt.Errorf("verifying topology case %s: %w", topology.Name, err)
		}
		if err := r.Cleanup(ctx); err != nil {
			return fmt.Errorf("cleaning topology case %s: %w", topology.Name, err)
		}
	}
	return nil
}

func (r *Runner) runTopologyCase(ctx context.Context, topology runconfig.TopologyCase) error {
	claimName := r.name("topology-" + topology.Name + "-claim")
	podName := r.name("topology-" + topology.Name + "-pod")
	requests := make([]resourcev1.DeviceRequest, 0, len(topology.Requests))
	for _, request := range topology.Requests {
		count := request.Count
		if count == 0 {
			count = 1
		}
		exact := &resourcev1.ExactDeviceRequest{DeviceClassName: request.DeviceClass, Count: count}
		if request.Selector != "" {
			exact.Selectors = []resourcev1.DeviceSelector{{CEL: &resourcev1.CELDeviceSelector{Expression: request.Selector}}}
		}
		requests = append(requests, resourcev1.DeviceRequest{Name: request.Name, Exactly: exact})
	}
	claim := &resourcev1.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{Name: claimName, Namespace: r.namespace, Labels: r.labels()},
		Spec:       resourcev1.ResourceClaimSpec{Devices: resourcev1.DeviceClaim{Requests: requests}},
	}
	if err := r.configureClaim(claim); err != nil {
		return err
	}
	if topology.MatchAttribute != "" {
		claim.Spec.Devices.Constraints = []resourcev1.DeviceConstraint{{
			Requests:       requestNames(topology.Requests),
			MatchAttribute: ptr(resourcev1.FullyQualifiedName(topology.MatchAttribute)),
		}}
	}
	var podClaims []corev1.PodResourceClaim
	if topology.UseTemplate {
		templateName := claimName + "-template"
		template := &resourcev1.ResourceClaimTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: templateName, Namespace: r.namespace, Labels: r.labels()},
			Spec:       resourcev1.ResourceClaimTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: r.labels()}, Spec: claim.Spec},
		}
		if _, err := r.clients.K8s.ResourceV1().ResourceClaimTemplates(r.namespace).Create(ctx, template, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("creating topology claim template %s: %w", templateName, err)
		}
		r.templates = append(r.templates, templateName)
		templateRef := templateName
		podClaims = []corev1.PodResourceClaim{{Name: "devices", ResourceClaimTemplateName: &templateRef}}
	} else {
		if _, err := r.clients.K8s.ResourceV1().ResourceClaims(r.namespace).Create(ctx, claim, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("creating topology claim %s: %w", claimName, err)
		}
		r.claims = append(r.claims, claimName)
		podClaims = []corev1.PodResourceClaim{{Name: "devices", ResourceClaimName: ptr(claimName)}}
	}
	pod := r.consumerPod(podName, podClaims)
	if _, err := r.clients.K8s.CoreV1().Pods(r.namespace).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("creating topology pod %s: %w", podName, err)
	}
	r.pods = append(r.pods, podName)
	if topology.UseTemplate {
		generated, err := r.waitTemplateClaim(ctx, podName)
		if err != nil {
			return err
		}
		claimName = generated
		r.claims = append(r.claims, claimName)
	}

	wantPending := topology.Expected == "pending"
	if wantPending {
		if err := r.waitPending(ctx, podName, claimName); err != nil {
			return fmt.Errorf("topology case %s: %w", topology.Name, err)
		}
		return nil
	}
	if err := r.waitAllocated(ctx, podName, []string{claimName}); err != nil {
		return fmt.Errorf("topology case %s: %w", topology.Name, err)
	}
	return nil
}

func (r *Runner) siblingExclusion(ctx context.Context) error {
	return r.siblingExclusionOrder(ctx, false)
}

func (r *Runner) siblingExclusionOrder(ctx context.Context, reverse bool) error {
	const class = "gpu.amd.com"
	if _, err := r.clients.K8s.ResourceV1().DeviceClasses().Get(ctx, class, metav1.GetOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			return errors.New("sibling-exclusion requires gpu.amd.com DeviceClass")
		}
		return err
	}
	firstType := "compute"
	firstSelector := "device.attributes[\"gpu.amd.com\"].type == 'amdgpu'"
	secondType := "vfio"
	secondSelector := "device.attributes[\"gpu.amd.com\"].type == 'vfio'"
	if reverse {
		firstType, secondType = secondType, firstType
		firstSelector, secondSelector = secondSelector, firstSelector
	}
	firstClaim := r.name("sibling-" + firstType + "-claim")
	firstPod := r.name("sibling-" + firstType + "-pod")
	claim, pod, err := r.createSelectedConsumer(ctx, firstClaim, firstPod, class, firstSelector)
	if err != nil {
		return err
	}
	if err := r.waitAllocated(ctx, pod, []string{claim}); err != nil {
		return fmt.Errorf("allocating compute sibling: %w", err)
	}
	pci, err := r.allocatedPCI(ctx, claim)
	if err != nil {
		return err
	}
	if pci == "" {
		return errors.New("sibling-exclusion could not find a PCI identity for the allocated compute device")
	}
	secondClaim := r.name("sibling-" + secondType + "-claim")
	secondPod := r.name("sibling-" + secondType + "-pod")
	vfio := secondSelector
	if pci != "" {
		vfio += " && " + pciExpression(pci)
	}
	second, secondConsumer, err := r.createSelectedConsumer(ctx, secondClaim, secondPod, class, vfio)
	if err != nil {
		return err
	}
	if err := r.waitPending(ctx, secondConsumer, second); err != nil {
		return err
	}
	return nil
}

func (r *Runner) alternateDevice(ctx context.Context) error {
	const class = "gpu.amd.com"
	compute := "device.attributes[\"gpu.amd.com\"].type == 'amdgpu'"
	vfio := "device.attributes[\"gpu.amd.com\"].type == 'vfio'"
	firstClaim, firstPod, err := r.createSelectedConsumer(ctx, r.name("alternate-compute-claim"), r.name("alternate-compute-pod"), class, compute)
	if err != nil {
		return err
	}
	if err := r.waitAllocated(ctx, firstPod, []string{firstClaim}); err != nil {
		return err
	}
	firstPCI, err := r.allocatedPCI(ctx, firstClaim)
	if err != nil {
		return err
	}
	secondClaim, secondPod, err := r.createSelectedConsumer(ctx, r.name("alternate-vfio-claim"), r.name("alternate-vfio-pod"), class, vfio)
	if err != nil {
		return err
	}
	if err := r.waitAllocated(ctx, secondPod, []string{secondClaim}); err != nil {
		return fmt.Errorf("allocating alternate VFIO device: %w", err)
	}
	secondPCI, err := r.allocatedPCI(ctx, secondClaim)
	if err != nil {
		return err
	}
	if firstPCI != "" && firstPCI == secondPCI {
		return fmt.Errorf("alternate VFIO claim selected the compute sibling at %s", firstPCI)
	}
	return nil
}

func (r *Runner) release(ctx context.Context) error {
	class, err := r.releaseClass(ctx)
	if err != nil {
		return err
	}
	claimName := r.name("release-claim")
	podName := r.name("release-pod")
	claim, pod, err := r.createSelectedConsumer(ctx, claimName, podName, class, "")
	if err != nil {
		return err
	}
	if err := r.waitAllocated(ctx, pod, []string{claim}); err != nil {
		return err
	}
	if err := r.deletePodAndWait(ctx, pod, []string{claim}); err != nil {
		return err
	}
	return nil
}

func (r *Runner) releaseOrders(ctx context.Context) error {
	const class = "gpu.amd.com"
	compute := "device.attributes[\"gpu.amd.com\"].type == 'amdgpu'"
	for order := 0; order < 2; order++ {
		firstClaim, firstPod, err := r.createSelectedConsumer(ctx, r.name(fmt.Sprintf("release-order-%d-first-claim", order)), r.name(fmt.Sprintf("release-order-%d-first-pod", order)), class, compute)
		if err != nil {
			return err
		}
		if err := r.waitAllocated(ctx, firstPod, []string{firstClaim}); err != nil {
			return err
		}
		secondClaim, secondPod, err := r.createSelectedConsumer(ctx, r.name(fmt.Sprintf("release-order-%d-second-claim", order)), r.name(fmt.Sprintf("release-order-%d-second-pod", order)), class, compute)
		if err != nil {
			return err
		}
		if err := r.waitAllocated(ctx, secondPod, []string{secondClaim}); err != nil {
			return err
		}
		firstPCI, err := r.allocatedPCI(ctx, firstClaim)
		if err != nil {
			return err
		}
		secondPCI, err := r.allocatedPCI(ctx, secondClaim)
		if err != nil {
			return err
		}
		if firstPCI != "" && firstPCI == secondPCI {
			return fmt.Errorf("release-order claims selected the same PCI identity %s", firstPCI)
		}

		if order == 0 {
			if err := r.deletePodAndWait(ctx, firstPod, []string{firstClaim}); err != nil {
				return fmt.Errorf("releasing first claim in A-then-B order: %w", err)
			}
			if err := r.waitAllocated(ctx, secondPod, []string{secondClaim}); err != nil {
				return fmt.Errorf("second claim was disturbed by A-then-B release: %w", err)
			}
			if err := r.deletePodAndWait(ctx, secondPod, []string{secondClaim}); err != nil {
				return err
			}
		} else {
			if err := r.deletePodAndWait(ctx, secondPod, []string{secondClaim}); err != nil {
				return fmt.Errorf("releasing second claim in B-then-A order: %w", err)
			}
			if err := r.waitAllocated(ctx, firstPod, []string{firstClaim}); err != nil {
				return fmt.Errorf("first claim was disturbed by B-then-A release: %w", err)
			}
			if err := r.deletePodAndWait(ctx, firstPod, []string{firstClaim}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Runner) releaseClass(ctx context.Context) (string, error) {
	candidates := make([]string, 0, len(r.targets)+4)
	for _, target := range r.targets {
		adapter, err := driver.Get(target.Name)
		if err == nil {
			candidates = append(candidates, adapter.DeviceClass())
		}
	}
	for _, name := range []string{"amd", "nvidia", "sriov", "cpu", "example"} {
		adapter, err := driver.Get(name)
		if err == nil {
			candidates = append(candidates, adapter.DeviceClass())
		}
	}
	seen := map[string]bool{}
	for _, class := range candidates {
		if seen[class] {
			continue
		}
		seen[class] = true
		if _, err := r.clients.K8s.ResourceV1().DeviceClasses().Get(ctx, class, metav1.GetOptions{}); err == nil {
			return class, nil
		} else if !apierrors.IsNotFound(err) {
			return "", fmt.Errorf("checking release DeviceClass %q: %w", class, err)
		}
	}
	return "", errors.New("release scenario found no registered driver DeviceClass")
}

func (r *Runner) capacity(ctx context.Context) error {
	class := "gpu.amd.com"
	if _, err := r.clients.K8s.ResourceV1().DeviceClasses().Get(ctx, class, metav1.GetOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			return errors.New("capacity requires gpu.amd.com DeviceClass")
		}
		return fmt.Errorf("checking capacity DeviceClass %q: %w", class, err)
	}
	maxClaims := 32
	allocated := 0
	for i := 0; i < maxClaims; i++ {
		claimName := r.name(fmt.Sprintf("capacity-%02d-claim", i))
		podName := r.name(fmt.Sprintf("capacity-%02d-pod", i))
		claim, pod, err := r.createSelectedConsumer(ctx, claimName, podName, class, "")
		if err != nil {
			return err
		}
		if err := r.waitAllocatedFor(ctx, pod, []string{claim}, pendingTimeout); err != nil {
			if err := r.waitPending(ctx, pod, claim); err != nil {
				return fmt.Errorf("capacity exhaustion did not produce a pending claim: %w", err)
			}
			break
		}
		allocated++
	}
	if allocated == 0 {
		return errors.New("capacity scenario allocated no claims")
	}
	if allocated == maxClaims {
		return fmt.Errorf("capacity scenario did not exhaust after %d claims", maxClaims)
	}
	return nil
}

func (r *Runner) restart(ctx context.Context, targets []DriverTarget) error {
	if err := r.restartDrivers(ctx, targets); err != nil {
		return err
	}
	return r.resourceSlices(ctx)
}

func (r *Runner) restartActive(ctx context.Context, targets []DriverTarget) error {
	const class = "gpu.amd.com"
	claim, pod, err := r.createSelectedConsumer(ctx, r.name("restart-active-claim"), r.name("restart-active-pod"), class, "device.attributes[\"gpu.amd.com\"].type == 'amdgpu'")
	if err != nil {
		return err
	}
	if err := r.waitAllocated(ctx, pod, []string{claim}); err != nil {
		return err
	}
	before, err := r.allocatedPCI(ctx, claim)
	if err != nil {
		return err
	}
	if err := r.restartDrivers(ctx, targets); err != nil {
		return err
	}
	if err := r.waitAllocated(ctx, pod, []string{claim}); err != nil {
		return fmt.Errorf("active claim was not preserved across restart: %w", err)
	}
	after, err := r.allocatedPCI(ctx, claim)
	if err != nil {
		return err
	}
	if before != "" && after != before {
		return fmt.Errorf("active claim changed PCI identity across restart: before %s, after %s", before, after)
	}
	if err := r.resourceSlices(ctx); err != nil {
		return fmt.Errorf("verifying ResourceSlices after active restart: %w", err)
	}
	if err := r.deletePodAndWait(ctx, pod, []string{claim}); err != nil {
		return fmt.Errorf("releasing active claim after restart: %w", err)
	}
	return nil
}

func (r *Runner) restartDrivers(ctx context.Context, targets []DriverTarget) error {
	if len(targets) == 0 {
		return errors.New("restart scenario has no driver targets")
	}
	for _, target := range targets {
		if target.Selector == "" {
			return fmt.Errorf("driver %s has no pod selector for restart", target.Name)
		}
		pods, err := r.clients.K8s.CoreV1().Pods(target.Namespace).List(ctx, metav1.ListOptions{LabelSelector: target.Selector})
		if err != nil {
			return fmt.Errorf("listing %s driver pods: %w", target.Name, err)
		}
		if len(pods.Items) == 0 {
			return fmt.Errorf("no pods matched %s selector %q", target.Name, target.Selector)
		}
		for _, pod := range pods.Items {
			if err := r.clients.K8s.CoreV1().Pods(target.Namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("restarting %s pod %s: %w", target.Name, pod.Name, err)
			}
		}
		if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, scenarioTimeout, true, func(ctx context.Context) (bool, error) {
			current, err := r.clients.K8s.CoreV1().Pods(target.Namespace).List(ctx, metav1.ListOptions{LabelSelector: target.Selector})
			if err != nil {
				return false, err
			}
			if len(current.Items) == 0 {
				return false, nil
			}
			for _, pod := range current.Items {
				if pod.Status.Phase != corev1.PodRunning {
					return false, nil
				}
				for _, status := range pod.Status.ContainerStatuses {
					if !status.Ready {
						return false, nil
					}
				}
			}
			return true, nil
		}); err != nil {
			return fmt.Errorf("waiting for %s driver restart: %w", target.Name, err)
		}
	}
	return nil
}

func (r *Runner) createSelectedConsumer(ctx context.Context, claimName, podName, class, selector string) (string, string, error) {
	exact := &resourcev1.ExactDeviceRequest{DeviceClassName: class}
	selector = r.defaultSelector(selector)
	if selector != "" {
		exact.Selectors = []resourcev1.DeviceSelector{{CEL: &resourcev1.CELDeviceSelector{Expression: selector}}}
	}
	claim := &resourcev1.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{Name: claimName, Namespace: r.namespace, Labels: r.labels()},
		Spec:       resourcev1.ResourceClaimSpec{Devices: resourcev1.DeviceClaim{Requests: []resourcev1.DeviceRequest{{Name: "device", Exactly: exact}}}},
	}
	if err := r.configureClaim(claim); err != nil {
		return "", "", err
	}
	if _, err := r.clients.K8s.ResourceV1().ResourceClaims(r.namespace).Create(ctx, claim, metav1.CreateOptions{}); err != nil {
		return "", "", fmt.Errorf("creating claim %s: %w", claimName, err)
	}
	r.claims = append(r.claims, claimName)
	pod := r.consumerPod(podName, []corev1.PodResourceClaim{{Name: "device", ResourceClaimName: ptr(claimName)}})
	if _, err := r.clients.K8s.CoreV1().Pods(r.namespace).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		return "", "", fmt.Errorf("creating pod %s: %w", podName, err)
	}
	r.pods = append(r.pods, podName)
	return claimName, podName, nil
}

func (r *Runner) consumerPod(name string, claims []corev1.PodResourceClaim) *corev1.Pod {
	image := defaultWorkloadImage
	if r.config.WorkloadImage != "" {
		image = r.config.WorkloadImage
	}
	containerClaims := make([]corev1.ResourceClaim, 0, len(claims))
	for _, claim := range claims {
		containerClaims = append(containerClaims, corev1.ResourceClaim{Name: claim.Name})
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: r.namespace, Labels: r.labels()},
		Spec: corev1.PodSpec{
			RestartPolicy:  corev1.RestartPolicyNever,
			ResourceClaims: claims,
			Containers:     []corev1.Container{{Name: "test", Image: image, Command: []string{"/bin/sh", "-ec", "sleep 600"}, Resources: corev1.ResourceRequirements{Claims: containerClaims}}},
		},
	}
}

func (r *Runner) defaultSelector(selector string) string {
	if selector != "" {
		return selector
	}
	return r.config.Selector
}

func (r *Runner) configureClaim(claim *resourcev1.ResourceClaim) error {
	if r.config.ClaimConfig == nil {
		return nil
	}
	parameters, err := json.Marshal(r.config.ClaimConfig.Parameters)
	if err != nil {
		return fmt.Errorf("encoding test-plan claim parameters: %w", err)
	}
	claim.Spec.Devices.Config = []resourcev1.DeviceClaimConfiguration{{
		Requests: r.config.ClaimConfig.Requests,
		DeviceConfiguration: resourcev1.DeviceConfiguration{Opaque: &resourcev1.OpaqueDeviceConfiguration{
			Driver:     r.config.ClaimConfig.Driver,
			Parameters: runtime.RawExtension{Raw: parameters},
		}},
	}}
	return nil
}

func (r *Runner) waitAllocated(ctx context.Context, podName string, claims []string) error {
	return r.waitAllocatedFor(ctx, podName, claims, scenarioTimeout)
}

func (r *Runner) waitAllocatedFor(ctx context.Context, podName string, claims []string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(ctx, 3*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		pod, err := r.clients.K8s.CoreV1().Pods(r.namespace).Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if pod.Status.Phase == corev1.PodFailed {
			return false, fmt.Errorf("pod %s failed: %s", podName, pod.Status.Message)
		}
		for _, claimName := range claims {
			claim, err := r.clients.K8s.ResourceV1().ResourceClaims(r.namespace).Get(ctx, claimName, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			if claim.Status.Allocation == nil {
				return false, nil
			}
		}
		return pod.Status.Phase == corev1.PodRunning || pod.Status.Phase == corev1.PodSucceeded, nil
	})
}

func (r *Runner) waitPending(ctx context.Context, podName, claimName string) error {
	started := time.Now()
	return wait.PollUntilContextTimeout(ctx, 3*time.Second, pendingTimeout+5*time.Second, true, func(ctx context.Context) (bool, error) {
		pod, err := r.clients.K8s.CoreV1().Pods(r.namespace).Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		switch pod.Status.Phase {
		case corev1.PodSucceeded, corev1.PodRunning:
			return false, fmt.Errorf("pod %s unexpectedly scheduled", podName)
		case corev1.PodFailed:
			return false, fmt.Errorf("pod %s failed while waiting for pending allocation: %s", podName, pod.Status.Message)
		case corev1.PodUnknown:
			return false, fmt.Errorf("pod %s entered unknown phase while waiting for pending allocation", podName)
		case corev1.PodPending:
			// Expected while the claim remains unallocated.
		}
		claim, err := r.clients.K8s.ResourceV1().ResourceClaims(r.namespace).Get(ctx, claimName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		return claim.Status.Allocation == nil && time.Since(started) >= pendingTimeout, nil
	})
}

func (r *Runner) waitTemplateClaim(ctx context.Context, podName string) (string, error) {
	var claimName string
	err := wait.PollUntilContextTimeout(ctx, 2*time.Second, scenarioTimeout, true, func(ctx context.Context) (bool, error) {
		pod, err := r.clients.K8s.CoreV1().Pods(r.namespace).Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if pod.Status.Phase == corev1.PodFailed {
			return false, fmt.Errorf("pod %s failed while generating ResourceClaim: %s", podName, pod.Status.Message)
		}
		for _, status := range pod.Status.ResourceClaimStatuses {
			if status.ResourceClaimName != nil && *status.ResourceClaimName != "" {
				claimName = *status.ResourceClaimName
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		return "", fmt.Errorf("waiting for ResourceClaim generated from pod %s: %w", podName, err)
	}
	return claimName, nil
}

func (r *Runner) deletePodAndWait(ctx context.Context, podName string, claims []string) error {
	if err := r.clients.K8s.CoreV1().Pods(r.namespace).Delete(ctx, podName, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("deleting pod %s: %w", podName, err)
	}
	return wait.PollUntilContextTimeout(ctx, 3*time.Second, scenarioTimeout, true, func(ctx context.Context) (bool, error) {
		for _, claimName := range claims {
			claim, err := r.clients.K8s.ResourceV1().ResourceClaims(r.namespace).Get(ctx, claimName, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				continue
			}
			if err != nil {
				return false, err
			}
			if claim.Status.Allocation != nil {
				return false, nil
			}
		}
		return true, nil
	})
}

func (r *Runner) allocatedPCI(ctx context.Context, claimName string) (string, error) {
	claim, err := r.clients.K8s.ResourceV1().ResourceClaims(r.namespace).Get(ctx, claimName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	if claim.Status.Allocation == nil || len(claim.Status.Allocation.Devices.Results) == 0 {
		return "", nil
	}
	result := claim.Status.Allocation.Devices.Results[0]
	slices, err := r.clients.K8s.ResourceV1().ResourceSlices().List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", err
	}
	for _, slice := range slices.Items {
		if slice.Spec.Driver != result.Driver || slice.Spec.Pool.Name != result.Pool {
			continue
		}
		for _, device := range slice.Spec.Devices {
			if device.Name != result.Device {
				continue
			}
			for key, value := range device.Attributes {
				if strings.HasSuffix(string(key), "pciBusID") || strings.HasSuffix(string(key), "pciAddress") || strings.HasSuffix(string(key), "pciAddr") {
					if value.StringValue == nil {
						continue
					}
					return string(key) + "=" + *value.StringValue, nil
				}
			}
		}
	}
	return "", nil
}

func (r *Runner) snapshot(ctx context.Context, prefix string) (verification.Snapshot, error) {
	if r.verify == nil {
		return verification.Snapshot{Prefix: prefix}, nil
	}
	return r.verify.SaveSnapshot(ctx, r.clients.K8s, prefix)
}

func (r *Runner) runDefaultVerifiers(ctx context.Context, prefix string) error {
	if !r.verify.Enabled() {
		return nil
	}
	if len(r.config.Verification.Commands) > 0 {
		return r.verify.RunConfigured(ctx, r.namespace, prefix)
	}
	commands := [][]string{{"dra-verify.sh", "attributes", "-a"}, {"dra-verify.sh", "topology"}, {"show-dra-topology.sh", "--summary"}}
	for _, command := range commands {
		name := prefix + "-" + command[0] + "-" + command[1]
		if err := r.verify.Run(ctx, name, r.namespace, command...); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) runVerifier(ctx context.Context, name string, args ...string) error {
	if !r.verify.Enabled() {
		return nil
	}
	return r.verify.Run(ctx, name, r.namespace, args...)
}

// Cleanup removes only resources with the runner's owner label. It is safe to
// call after a partially completed scenario.
func (r *Runner) Cleanup(ctx context.Context) error {
	var problems []error
	for _, pod := range r.pods {
		if err := r.clients.K8s.CoreV1().Pods(r.namespace).Delete(ctx, pod, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			problems = append(problems, fmt.Errorf("deleting test pod %s: %w", pod, err))
		}
	}
	// DRA keeps a claim reserved until the consuming pod has disappeared.
	// Poll one labeled list rather than one GET per pod: capacity scenarios can
	// own several consumers and the Kubernetes client rate limiter otherwise
	// spends the cleanup budget on redundant requests.
	if len(r.pods) > 0 {
		if err := wait.PollUntilContextTimeout(ctx, 2*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
			pods, err := r.clients.K8s.CoreV1().Pods(r.namespace).List(ctx, metav1.ListOptions{LabelSelector: "dra-harness/run=" + r.owner})
			if err != nil {
				if apierrors.IsNotFound(err) {
					return true, nil
				}
				return false, err
			}
			return len(pods.Items) == 0, nil
		}); err != nil {
			problems = append(problems, fmt.Errorf("waiting for test pod deletion: %w", err))
		}
	}
	// Wait for all owned claims to lose their allocation before deleting them.
	// This is also list-based to keep cleanup bounded for capacity scenarios.
	if len(r.claims) > 0 {
		if err := wait.PollUntilContextTimeout(ctx, 2*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
			claims, err := r.clients.K8s.ResourceV1().ResourceClaims(r.namespace).List(ctx, metav1.ListOptions{LabelSelector: "dra-harness/run=" + r.owner})
			if err != nil {
				if apierrors.IsNotFound(err) {
					return true, nil
				}
				return false, err
			}
			for _, claim := range claims.Items {
				if claim.Status.Allocation != nil {
					return false, nil
				}
			}
			return true, nil
		}); err != nil {
			problems = append(problems, fmt.Errorf("waiting for test claims release: %w", err))
		}
	}
	for _, claim := range r.claims {
		if err := r.clients.K8s.ResourceV1().ResourceClaims(r.namespace).Delete(ctx, claim, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			problems = append(problems, fmt.Errorf("deleting test claim %s: %w", claim, err))
		}
	}
	for _, template := range r.templates {
		if err := r.clients.K8s.ResourceV1().ResourceClaimTemplates(r.namespace).Delete(ctx, template, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			problems = append(problems, fmt.Errorf("deleting test claim template %s: %w", template, err))
		}
	}
	return errors.Join(problems...)
}

func (r *Runner) labels() map[string]string {
	return map[string]string{"app.kubernetes.io/created-by": "dra-harness-testplan", "dra-harness/run": r.owner}
}

func (r *Runner) name(prefix string) string {
	return strings.Trim(strings.ToLower(prefix+"-"+rand.String(5)), "-")
}

func requestNames(requests []runconfig.TopologyRequest) []string {
	names := make([]string, 0, len(requests))
	for _, request := range requests {
		names = append(names, request.Name)
	}
	return names
}

func pciExpression(value string) string {
	parts := strings.SplitN(value, "=", 2)
	if len(parts) != 2 {
		return "false"
	}
	key := parts[0]
	idx := strings.LastIndex(key, "/")
	domain, attr := key, key
	if idx >= 0 {
		domain, attr = key[:idx], key[idx+1:]
	}
	return fmt.Sprintf("device.attributes[%q].%s == %q", domain, attr, parts[1])
}

func ptr[T any](value T) *T { return &value }
