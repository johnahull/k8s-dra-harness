// Package platform isolates every OpenShift/Kubernetes difference.
package platform

import (
	"fmt"

	"github.com/johnahull/amd-ci/internal/config"
	"k8s.io/client-go/discovery"
)

// openShiftConfigGroup is served by every OpenShift cluster and by no vanilla Kubernetes cluster.
const openShiftConfigGroup = "config.openshift.io"

// Detect returns override when it is set, otherwise asks the API server which
// groups it serves.
func Detect(dc discovery.ServerGroupsInterface, override config.Platform) (config.Platform, error) {
	if override != "" {
		switch override {
		case config.PlatformOpenShift, config.PlatformKubernetes:
			return override, nil
		default:
			return "", fmt.Errorf("invalid AMD_PLATFORM %q: must be %q or %q",
				override, config.PlatformOpenShift, config.PlatformKubernetes)
		}
	}

	groups, err := dc.ServerGroups()
	if err != nil {
		return "", fmt.Errorf("listing API groups: %w", err)
	}

	for _, g := range groups.Groups {
		if g.Name == openShiftConfigGroup {
			return config.PlatformOpenShift, nil
		}
	}

	return config.PlatformKubernetes, nil
}
