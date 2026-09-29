package testplan

import (
	"testing"

	"github.com/johnahull/k8s-dra-harness/internal/runconfig"
	corev1 "k8s.io/api/core/v1"
)

func TestConsumerPodUsesOnePodClaimForMultipleRequests(t *testing.T) {
	runner := &Runner{config: &runconfig.TestPlan{WorkloadImage: "example/test"}, namespace: "workload"}
	name := "claim"
	pod := runner.consumerPod("pod", []corev1.PodResourceClaim{{Name: "devices", ResourceClaimName: &name}})
	if len(pod.Spec.ResourceClaims) != 1 || pod.Spec.ResourceClaims[0].Name != "devices" {
		t.Fatalf("pod resource claims = %+v", pod.Spec.ResourceClaims)
	}
	if len(pod.Spec.Containers[0].Resources.Claims) != 1 || pod.Spec.Containers[0].Resources.Claims[0].Name != "devices" {
		t.Fatalf("container resource claims = %+v", pod.Spec.Containers[0].Resources.Claims)
	}
	if pod.Spec.Containers[0].Image != "example/test" {
		t.Fatalf("image=%q", pod.Spec.Containers[0].Image)
	}
}

func TestPCIExpression(t *testing.T) {
	got := pciExpression("resource.kubernetes.io/pciBusID=0000:01:00.0")
	want := `device.attributes["resource.kubernetes.io"].pciBusID == "0000:01:00.0"`
	if got != want {
		t.Fatalf("pciExpression()=%q, want %q", got, want)
	}
}
