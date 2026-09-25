// Package defaults registers the adapters included with the harness test suite.
package defaults

import (
	_ "github.com/johnahull/k8s-dra-harness/internal/driver/amd"
	_ "github.com/johnahull/k8s-dra-harness/internal/driver/cpu"
	_ "github.com/johnahull/k8s-dra-harness/internal/driver/example"
)
