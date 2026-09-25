package driver

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

type jointPolicy func([]Adapter) corev1.Container

var jointPolicies = map[[2]string]jointPolicy{}

func jointKey(first, second string) [2]string {
	if first > second {
		first, second = second, first
	}
	return [2]string{first, second}
}

// RegisterJoint adds a live check for a pair of different drivers.
func RegisterJoint(first, second string, policy func([]Adapter) corev1.Container) {
	if first == second || first == "" || second == "" || policy == nil {
		panic("invalid joint driver policy")
	}
	key := jointKey(first, second)
	if _, exists := jointPolicies[key]; exists {
		panic("duplicate joint driver policy")
	}
	jointPolicies[key] = policy
}

// SupportsJoint reports whether the selected adapters have a combined check.
func SupportsJoint(adapters []Adapter) bool {
	if len(adapters) != 2 || adapters[0] == nil || adapters[1] == nil || adapters[0].Name() == adapters[1].Name() {
		return false
	}
	_, ok := jointPolicies[jointKey(adapters[0].Name(), adapters[1].Name())]
	return ok
}

// JointWorkload builds the registered combined workload for two adapters.
func JointWorkload(adapters []Adapter) (corev1.Container, error) {
	if !SupportsJoint(adapters) {
		return corev1.Container{}, fmt.Errorf("no joint workload registered for selected drivers")
	}
	return jointPolicies[jointKey(adapters[0].Name(), adapters[1].Name())](adapters), nil
}
