package harness

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/johnahull/k8s-dra-harness/internal/driver"
	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

const checkTimeout = 5 * time.Minute

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

// RunJointWorkload allocates AMD GPU and CPU claims to a single pod.
func (r *Runner) RunJointWorkload(ctx context.Context) error {
	if len(r.Drivers) != 2 {
		return fmt.Errorf("joint workload requires exactly AMD and CPU")
	}
	var amd, cpu *Installed
	for i := range r.Drivers {
		switch r.Drivers[i].Config.Name {
		case amdName:
			amd = &r.Drivers[i]
		case cpuName:
			cpu = &r.Drivers[i]
		}
	}
	if amd == nil || cpu == nil {
		return fmt.Errorf("joint workload requires AMD and CPU")
	}
	return r.runWorkload(ctx, []Installed{*amd, *cpu})
}

// RunOperatorWorkload checks an operator-only installation through the AMD
// device plugin and a real GPU workload.
func (r *Runner) RunOperatorWorkload(ctx context.Context) error {
	if r.Config.Operator == nil || len(r.Drivers) != 0 {
		return fmt.Errorf("operator workload requires an operator-only run")
	}
	var last string
	err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 30*time.Minute, true, func(ctx context.Context) (bool, error) {
		nodes, err := r.Client.K8s.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		if err != nil {
			return false, err
		}
		for _, n := range nodes.Items {
			quantity := n.Status.Allocatable[corev1.ResourceName("amd.com/gpu")]
			if quantity.Value() > 0 {
				return true, nil
			}
		}
		last = "no node advertises amd.com/gpu"
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("waiting for AMD device plugin (%s): %w", last, err)
	}
	ns, name := r.WorkloadNamespace(), "amd-operator-"+r.ID
	limits := corev1.ResourceList{corev1.ResourceName("amd.com/gpu"): resource.MustParse("1")}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}, Spec: corev1.PodSpec{RestartPolicy: corev1.RestartPolicyNever,
		Containers: []corev1.Container{{Name: "test", Image: "docker.io/rocm/dev-ubuntu-22.04:6.4", Command: []string{"/bin/sh", "-ec", "rocm-smi && echo PASS"}, Resources: corev1.ResourceRequirements{Limits: limits}}}}}
	if _, err := r.Client.K8s.CoreV1().Pods(ns).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		return err
	}
	defer func() { _ = r.Client.K8s.CoreV1().Pods(ns).Delete(context.Background(), name, metav1.DeleteOptions{}) }()
	err = wait.PollUntilContextTimeout(ctx, 5*time.Second, 10*time.Minute, true, func(ctx context.Context) (bool, error) {
		p, err := r.Client.K8s.CoreV1().Pods(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		last = fmt.Sprintf("phase=%s reason=%s message=%s", p.Status.Phase, p.Status.Reason, p.Status.Message)
		switch p.Status.Phase {
		case corev1.PodSucceeded:
			return true, nil
		case corev1.PodFailed:
			return false, fmt.Errorf("operator workload failed: %s", last)
		case corev1.PodPending, corev1.PodRunning, corev1.PodUnknown:
			return false, nil
		default:
			return false, fmt.Errorf("unrecognized pod phase %q", p.Status.Phase)
		}
	})
	if err != nil {
		return fmt.Errorf("waiting for operator workload (%s): %w", last, err)
	}
	stream, err := r.Client.K8s.CoreV1().Pods(ns).GetLogs(name, &corev1.PodLogOptions{Container: "test"}).Stream(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = stream.Close() }()
	buf := new(strings.Builder)
	if _, err := io.Copy(buf, stream); err != nil {
		return err
	}
	if !strings.Contains(buf.String(), "PASS") {
		return fmt.Errorf("operator workload logs lack PASS: %s", buf.String())
	}
	return nil
}

func (r *Runner) runWorkload(ctx context.Context, selected []Installed) error {
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
	primary := selected[0].Adapter
	command := primary.WorkloadCommand()
	if len(selected) == 2 {
		primary, _ = driver.Get(amdName)
		command = []string{"/bin/sh", "-ec", "rocm-smi && env | grep '^DRA_CPUSET_' && echo PASS"}
	}
	container := corev1.Container{Name: "test", Image: primary.WorkloadImage(), Command: command}
	podClaims := make([]corev1.PodResourceClaim, 0, len(claims))
	for i, claim := range claims {
		key := selected[i].Config.Name
		podClaims = append(podClaims, corev1.PodResourceClaim{Name: key, ResourceClaimName: &claim})
		container.Resources.Claims = append(container.Resources.Claims, corev1.ResourceClaim{Name: key})
	}
	if len(selected) == 1 && selected[0].Config.Name == cpuName || len(selected) == 2 {
		// The CPU driver's NRI integration requires a Guaranteed QoS pod.
		container.Resources.Requests = corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1"), corev1.ResourceMemory: resource.MustParse("64Mi")}
		container.Resources.Limits = corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1"), corev1.ResourceMemory: resource.MustParse("64Mi")}
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
			return true, nil
		case corev1.PodFailed:
			return false, fmt.Errorf("workload failed: %s", last)
		case corev1.PodPending, corev1.PodRunning, corev1.PodUnknown:
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
	for _, claimName := range claims {
		claim, err := r.Client.K8s.ResourceV1().ResourceClaims(ns).Get(ctx, claimName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if claim.Status.Allocation == nil {
			return fmt.Errorf("claim %s was not allocated", claimName)
		}
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
