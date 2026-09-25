package harness

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/johnahull/k8s-dra-harness/internal/amdoperator"
	"github.com/johnahull/k8s-dra-harness/internal/driver"
	"github.com/johnahull/k8s-dra-harness/internal/nvidiaoperator"
	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

const checkTimeout = 5 * time.Minute

// ValidateExisting verifies selected drivers without creating or deleting any
// Kubernetes resources. It is intended for shared clusters where the driver
// is already installed outside this run.
func (r *Runner) ValidateExisting(ctx context.Context) error {
	if !r.Config.Existing {
		return fmt.Errorf("existing-driver validation requires existing: true")
	}
	if _, err := r.Client.Discovery.ServerResourcesForGroupVersion("resource.k8s.io/v1"); err != nil {
		return fmt.Errorf("cluster must serve resource.k8s.io/v1: %w", err)
	}
	r.Drivers = nil
	for _, config := range r.Config.Drivers {
		adapter, err := driver.Get(config.Name)
		if err != nil {
			return err
		}
		installed := Installed{Adapter: adapter, Config: config, Namespace: config.Namespace}
		if _, err := r.Client.K8s.ResourceV1().DeviceClasses().Get(ctx, adapter.DeviceClass(), metav1.GetOptions{}); err != nil {
			return fmt.Errorf("existing driver %s DeviceClass %q: %w", config.Name, adapter.DeviceClass(), err)
		}
		slices, err := r.Client.K8s.ResourceV1().ResourceSlices().List(ctx, metav1.ListOptions{})
		if err != nil {
			return fmt.Errorf("listing ResourceSlices for existing driver %s: %w", config.Name, err)
		}
		found := false
		for _, slice := range slices.Items {
			if slice.Spec.Driver == adapter.DriverName() && len(slice.Spec.Devices) > 0 {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("existing driver %s has no nonempty ResourceSlices for %q", config.Name, adapter.DriverName())
		}
		r.Drivers = append(r.Drivers, installed)
	}
	return nil
}

// CheckDriver waits for the driver's DeviceClass, ResourceSlices, and pods.
func (r *Runner) CheckDriver(ctx context.Context, d Installed) error {
	var last string
	err := wait.PollUntilContextTimeout(ctx, 5*time.Second, checkTimeout, true, func(ctx context.Context) (bool, error) {
		if _, err := r.Client.K8s.ResourceV1().DeviceClasses().Get(ctx, d.Adapter.DeviceClass(), metav1.GetOptions{}); err != nil {
			if !apierrors.IsNotFound(err) {
				return false, err
			}
			last = "DeviceClass not present"
			return false, nil
		}
		slices, err := r.Client.K8s.ResourceV1().ResourceSlices().List(ctx, metav1.ListOptions{})
		if err != nil {
			return false, err
		}
		found := false
		for _, s := range slices.Items {
			if s.Spec.Driver == d.Adapter.DriverName() && len(s.Spec.Devices) > 0 {
				found = true
				break
			}
		}
		if !found {
			last = "no nonempty ResourceSlices"
			return false, nil
		}
		pods, err := r.Client.K8s.CoreV1().Pods(d.Namespace).List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/instance=" + d.Release})
		if err != nil {
			return false, err
		}
		if len(pods.Items) == 0 {
			last = "no driver pods with Helm release label"
			return false, nil
		}
		running := 0
		for _, p := range pods.Items {
			if p.Status.Phase == corev1.PodSucceeded {
				continue
			}
			if p.Status.Phase != corev1.PodRunning {
				last = fmt.Sprintf("pod %s is %s", p.Name, p.Status.Phase)
				return false, nil
			}
			running++
			for _, c := range p.Status.ContainerStatuses {
				if !c.Ready {
					last = fmt.Sprintf("container %s in pod %s is not ready", c.Name, p.Name)
					return false, nil
				}
			}
		}
		if running == 0 {
			last = "no running driver pods"
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("driver %s did not become ready (%s): %w", d.Config.Name, last, err)
	}
	return nil
}

// RunWorkload allocates one device and proves the workload can use it.
func (r *Runner) RunWorkload(ctx context.Context, d Installed) error {
	return r.runWorkload(ctx, []Installed{d})
}

// SupportsJointWorkload reports whether the selected pair has a combined check.
func (r *Runner) SupportsJointWorkload() bool {
	adapters := make([]driver.Adapter, 0, len(r.Drivers))
	for _, selected := range r.Drivers {
		adapters = append(adapters, selected.Adapter)
	}
	return driver.SupportsJoint(adapters)
}

// RunJointWorkload allocates a registered driver pair to a single pod.
func (r *Runner) RunJointWorkload(ctx context.Context) error {
	return r.runWorkload(ctx, r.Drivers)
}

// RunOperatorWorkload checks an operator-only installation through its device
// plugin and a real GPU workload.
func (r *Runner) RunOperatorWorkload(ctx context.Context) error {
	if (r.Config.Operator == nil && r.Config.NVIDIAOperator == nil) || len(r.Drivers) != 0 {
		return fmt.Errorf("operator workload requires an operator-only run")
	}
	if r.Config.NVIDIAOperator != nil {
		return nvidiaoperator.RunWorkload(ctx, r.Client, r.WorkloadNamespace(), r.ID)
	}
	return amdoperator.RunWorkload(ctx, r.Client, r.WorkloadNamespace(), r.ID)
}

func (r *Runner) runWorkload(ctx context.Context, selected []Installed) error {
	if len(selected) == 0 || len(selected) > 2 {
		return fmt.Errorf("workload requires one driver or the supported driver pair")
	}
	container := selected[0].Adapter.Workload()
	if len(selected) == 2 {
		var err error
		container, err = driver.JointWorkload([]driver.Adapter{selected[0].Adapter, selected[1].Adapter})
		if err != nil {
			return err
		}
	}
	ns := r.WorkloadNamespace()
	name := "dra-workload-" + r.ID
	if len(selected) == 2 {
		name = "dra-joint-" + r.ID
	}
	claims := make([]string, 0, len(selected))
	defer func() {
		for _, claim := range claims {
			_ = r.Client.K8s.ResourceV1().ResourceClaims(ns).Delete(context.Background(), claim, metav1.DeleteOptions{})
		}
	}()
	for _, d := range selected {
		claimName := name + "-" + d.Config.Name
		claim := &resourcev1.ResourceClaim{ObjectMeta: metav1.ObjectMeta{Name: claimName, Namespace: ns},
			Spec: resourcev1.ResourceClaimSpec{Devices: resourcev1.DeviceClaim{Requests: []resourcev1.DeviceRequest{{Name: "device", Exactly: &resourcev1.ExactDeviceRequest{DeviceClassName: d.Adapter.DeviceClass()}}}}}}
		if _, err := r.Client.K8s.ResourceV1().ResourceClaims(ns).Create(ctx, claim, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("creating %s claim: %w", d.Config.Name, err)
		}
		claims = append(claims, claimName)
	}
	podClaims := make([]corev1.PodResourceClaim, 0, len(claims))
	for i, claim := range claims {
		key := selected[i].Config.Name
		podClaims = append(podClaims, corev1.PodResourceClaim{Name: key, ResourceClaimName: &claim})
		container.Resources.Claims = append(container.Resources.Claims, corev1.ResourceClaim{Name: key})
	}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}, Spec: corev1.PodSpec{RestartPolicy: corev1.RestartPolicyNever, ResourceClaims: podClaims, Containers: []corev1.Container{container}}}
	if _, err := r.Client.K8s.CoreV1().Pods(ns).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("creating workload pod: %w", err)
	}
	defer func() { _ = r.Client.K8s.CoreV1().Pods(ns).Delete(context.Background(), name, metav1.DeleteOptions{}) }()
	var last string
	err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 10*time.Minute, true, func(ctx context.Context) (bool, error) {
		p, err := r.Client.K8s.CoreV1().Pods(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		last = fmt.Sprintf("pod phase=%s reason=%s message=%s", p.Status.Phase, p.Status.Reason, p.Status.Message)
		switch p.Status.Phase {
		case corev1.PodSucceeded:
			// A completed pod proves that the scheduler and kubelet accepted
			// the DRA claims. Some clusters clear or stop exposing allocation
			// status as the pod exits, so do not require a post-completion read.
			return true, nil
		case corev1.PodFailed:
			return false, fmt.Errorf("workload failed: %s", last)
		case corev1.PodPending, corev1.PodRunning, corev1.PodUnknown:
			for _, claimName := range claims {
				claim, err := r.Client.K8s.ResourceV1().ResourceClaims(ns).Get(ctx, claimName, metav1.GetOptions{})
				if err != nil {
					return false, err
				}
				if claim.Status.Allocation == nil {
					last = fmt.Sprintf("pod phase=%s; claim %s is not allocated", p.Status.Phase, claimName)
					return false, nil
				}
			}
			return false, nil
		default:
			return false, fmt.Errorf("unrecognized pod phase %q", p.Status.Phase)
		}
	})
	if err != nil {
		return fmt.Errorf("waiting for workload (%s): %w", last, err)
	}
	logs := r.Client.K8s.CoreV1().Pods(ns).GetLogs(name, &corev1.PodLogOptions{Container: "test"})
	stream, err := logs.Stream(ctx)
	if err != nil {
		return fmt.Errorf("reading workload logs: %w", err)
	}
	defer func() { _ = stream.Close() }()
	buf := new(strings.Builder)
	if _, err := io.Copy(buf, stream); err != nil {
		return err
	}
	if !strings.Contains(buf.String(), "PASS") {
		return fmt.Errorf("workload logs lack PASS: %s", buf.String())
	}
	if err := r.Client.K8s.CoreV1().Pods(ns).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		return err
	}
	return wait.PollUntilContextTimeout(ctx, 5*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		for _, claimName := range claims {
			claim, err := r.Client.K8s.ResourceV1().ResourceClaims(ns).Get(ctx, claimName, metav1.GetOptions{})
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
