package nvidiaoperator

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/johnahull/k8s-dra-harness/pkg/clients"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

// RunWorkload verifies the operator device plugin with a live GPU pod.
func RunWorkload(ctx context.Context, client *clients.Settings, namespace, id string) error {
	var last string
	err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 30*time.Minute, true, func(ctx context.Context) (bool, error) {
		nodes, err := client.K8s.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		if err != nil {
			return false, err
		}
		for _, n := range nodes.Items {
			quantity := n.Status.Allocatable[corev1.ResourceName("nvidia.com/gpu")]
			if quantity.Value() > 0 {
				return true, nil
			}
		}
		last = "no node advertises nvidia.com/gpu"
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("waiting for NVIDIA device plugin (%s): %w", last, err)
	}
	ns, name := namespace, "nvidia-operator-"+id
	pod := WorkloadPod(ns, name)
	if _, err := client.K8s.CoreV1().Pods(ns).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		return err
	}
	defer func() { _ = client.K8s.CoreV1().Pods(ns).Delete(context.Background(), name, metav1.DeleteOptions{}) }()
	err = wait.PollUntilContextTimeout(ctx, 5*time.Second, 10*time.Minute, true, func(ctx context.Context) (bool, error) {
		p, err := client.K8s.CoreV1().Pods(ns).Get(ctx, name, metav1.GetOptions{})
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
	stream, err := client.K8s.CoreV1().Pods(ns).GetLogs(name, &corev1.PodLogOptions{Container: "test"}).Stream(ctx)
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
