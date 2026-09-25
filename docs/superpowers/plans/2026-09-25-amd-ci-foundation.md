# amd-ci Foundation Implementation Plan (Plan 1 of 3)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the foundation of the `amd-ci` framework: repo tooling, env-var config with platform defaults and validation, OpenShift/Kubernetes detection, API clients, a typed `DeviceConfig` builder, and GPU discovery. It ends with a `smoke` Ginkgo suite that runs against a real cluster.

**Architecture:** A Go module structured like nvidia-ci (`internal/` helpers, `pkg/` resource wrappers, `tests/` Ginkgo suites, dot-imported `inittools` globals). Platform detection checks whether the cluster serves the `config.openshift.io` API group. `DeviceConfig` uses a minimal local copy of AMD's `amd.com/v1alpha1` types and is only ever *updated* through JSON merge patches, so fields this repo doesn't model are never overwritten.

**Tech Stack:** Go 1.25.5, Ginkgo v2.28.1 / Gomega v1.39.1, controller-runtime v0.23.1, k8s.io/* v0.35.1, kelseyhightower/envconfig v1.4.0, golang/glog v1.2.5, golangci-lint v2.8.0.

**Spec:** `docs/superpowers/specs/2026-09-25-amd-ci-design.md`

**Plan series:**
- **Plan 1 (this):** foundation plus the `smoke` suite.
- **Plan 2:** `Platform` interface + OpenShift/Kubernetes implementations (OLM, Helm, NFD, KMM, SCC), `tests/amdgpu` deploy + workload suite, reporter, must-gather, Containerfile, `DRY_RUN`.
- **Plan 3:** `tests/dra` suite (operator and Helm DRA sources, claims, sharing, negative test).

**Conventions for every task:**
- Repo root: `~/redhat/amd/amd-ci` (already `git init`ed, with the spec committed).
- File names must match `^[a-z][a-z0-9]*(_suite_test|_test)?\.go$` (the revive rule copied from nvidia-ci). No underscores or dashes elsewhere in the name.
- Dependencies are vendored. After adding an import of a new module, run `make deps-update`.
- Unit tests use the standard `testing` package. Only `tests/` uses Ginkgo.
- Commit messages end with `Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>` (this superseded the `Claude Opus 5.5` trailer shown above partway through implementation — Tasks 1-8's commits already use the Sonnet 5 trailer; use it for all remaining commits too).

---

## Progress (updated 2026-09-25)

**Tasks 1-11 are DONE.** Tasks 9 and 10 were committed as `e0bb5f4` and
`e4e29cc`. Task 11 reconciles the design spec in the final foundation commit.
`go test ./...` and `make verify` passed after Tasks 9 and 10. The smoke suite
compiled with the `integration` tag, but its live run was not completed: the
local kubeconfig references missing Minikube certificates.

Repo: `~/redhat/amd/amd-ci`, all work committed directly to `main` (explicitly approved by the user for this repo — no worktree/feature branch was used). Latest commit as of this handoff: `fd77b79` ("feat(amdgpu): add DeviceConfig builder with merge-patch updates").

Tasks 1-8 used the `superpowers:subagent-driven-development` workflow, with
spec and code-quality reviews after each task. Tasks 9-11 were implemented
directly, with diff reviews and the required gates after each task.

**To continue:** begin Plan 2. Run the smoke suite with a valid cluster
kubeconfig when one is available.

**Deferred, non-blocking follow-ups** (found by code-quality review, judged not worth unwinding the pipeline for — see each task's status note above for full detail, tracked here so they aren't lost):
1. **Task 2** (`internal/config`): thin env-var test coverage; a reflection-based safeguard test would fix several related gaps at once.
2. **Task 5** (`internal/platform`): `Detect` and `Config.Validate` duplicate `Platform` enum validation with different error text — needs a real design decision, not a mechanical fix (see task note for why).
3. **Task 8** (`pkg/amdgpu` builder): no way to explicitly clear a string/map field via `With*` — a design decision needed before Plan 2's deploy suite, not a bug.

The earlier `goconst` lint finding was fixed in Task 9. The remaining items
can be handled in a later hardening pass.

---

## File map

| File | Responsibility |
|---|---|
| `go.mod`, `Makefile`, `.golangci.yml`, `.gitignore`, `README.md` | Tooling |
| `scripts/common.sh`, `scripts/golangci-lint.sh` | Lint installer (copied from nvidia-ci) |
| `scripts/test-runner.sh` | `TEST_FEATURES`/`TEST_LABELS` → ginkgo |
| `internal/config/config.go` | `Config` struct, enums, `Load()` |
| `internal/config/keyvalues.go` | `KeyValues` env decoder for `k=v,k=v` |
| `internal/config/defaults.go` | `ApplyPlatformDefaults()`, `Timeout()` |
| `internal/config/validate.go` | `Validate()` |
| `internal/platform/detect.go` | `Detect()` OpenShift vs Kubernetes |
| `pkg/amdgpu/v1alpha1/types.go` | Minimal `DeviceConfig` types |
| `pkg/amdgpu/v1alpha1/register.go` | Scheme registration, deep copy |
| `pkg/clients/clients.go` | `Settings`, `NewScheme()`, `New()` |
| `pkg/amdgpu/builder.go` | `DeviceConfig` builder (Create/Pull/Apply/Delete/Exists) |
| `internal/discovery/nodes.go` | `IsAMDGPUNode()`, `GPUNodes()` |
| `internal/discovery/slices.go` | `DevicesByNode()` from ResourceSlices |
| `internal/inittools/inittools.go` | `APIClient`, `Config` globals |
| `tests/smoke/smoke_suite_test.go`, `tests/smoke/smoke_test.go` | Smoke suite |

---

### Task 1: Repo scaffold and tooling

> **Status: ✅ DONE** — commit `c488b21`. Spec review passed, quality review passed (no issues).

**Files:**
- Create: `go.mod`, `Makefile`, `.golangci.yml`, `.gitignore`, `README.md`, `scripts/common.sh`, `scripts/golangci-lint.sh`, `scripts/test-runner.sh`

- [ ] **Step 1: Initialize the module**

```bash
cd ~/redhat/amd/amd-ci
go mod init github.com/johnahull/amd-ci
go mod edit -go=1.25.5
```

- [ ] **Step 2: Copy the lint config and lint scripts from nvidia-ci**

```bash
cp ~/redhat/nvidia/nvidia-ci/.golangci.yml .golangci.yml
mkdir -p scripts
cp ~/redhat/nvidia/nvidia-ci/scripts/common.sh scripts/common.sh
cp ~/redhat/nvidia/nvidia-ci/scripts/golangci-lint.sh scripts/golangci-lint.sh
chmod +x scripts/*.sh
```

- [ ] **Step 3: Write `Makefile`**

```make
export GO111MODULE=on
MODULE := github.com/johnahull/amd-ci
GO_PACKAGES = $(shell go list ./... | grep -v /vendor/)
# Packages under tests/ are Ginkgo suites that need a cluster; unit-test skips them.
TEST ?= ...
ARGS ?=

.PHONY: help vet lint verify deps-update unit-test install-ginkgo run-tests

help: ## Show available make targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

vet: ## Run go vet
	go vet $(GO_PACKAGES)

lint: ## Run golangci-lint (installs v2.8.0 if needed)
	scripts/golangci-lint.sh

verify: lint vet ## Lint + vet

deps-update: ## go mod tidy && go mod vendor
	go mod tidy && go mod vendor

unit-test: ## Run unit tests (TEST=pkg/amdgpu to narrow)
	go test $$(go list $(MODULE)/$(TEST) | grep -v '/tests/')

install-ginkgo: ## Install the ginkgo CLI
	go install github.com/onsi/ginkgo/v2/ginkgo@v2.28.1

run-tests: ## Run Ginkgo suites (TEST_FEATURES required)
	scripts/test-runner.sh $(ARGS)
```

- [ ] **Step 4: Write `scripts/test-runner.sh`**

```bash
#!/usr/bin/env bash
# Runs the Ginkgo suites under tests/ selected by TEST_FEATURES (space- or
# comma-separated directory names, or "all") and filtered by TEST_LABELS.

GOPATH="${GOPATH:-${HOME}/go}"
PATH=$PATH:$GOPATH/bin
TEST_DIR="./tests"

if [[ -n "${ARTIFACT_DIR}" ]]; then
    export REPORTS_DUMP_DIR=${ARTIFACT_DIR}
fi

if [[ -z "${TEST_FEATURES}" ]]; then
    echo "TEST_FEATURES environment variable is undefined"
    exit 1
fi

if [[ "${TEST_FEATURES}" == "all" ]]; then
    feature_dirs=${TEST_DIR}
else
    for feature in ${TEST_FEATURES//,/ }; do
        discovered=$(find $TEST_DIR -depth -type d -name "${feature}" 2> /dev/null)
        if [[ -n $discovered ]]; then
            feature_dirs+=" "$discovered
        elif [[ "${VERBOSE_SCRIPT}" == "true" ]]; then
            echo "Could not find any feature directories matching ${feature}"
        fi
    done

    if [[ -z "${feature_dirs}" ]]; then
        echo "Could not find any feature directories for provided features: ${TEST_FEATURES}"
        exit 1
    fi
fi

cmd="ginkgo -timeout=24h --keep-going --require-suite -r"

if [[ "${TEST_VERBOSE}" == "true" ]]; then
    cmd+=" -vv"
fi

if [[ "${TEST_TRACE}" == "true" ]]; then
    cmd+=" --trace"
fi

if [[ -n "${TEST_LABELS}" ]]; then
    cmd+=" --label-filter=\"${TEST_LABELS}\""
fi
cmd+=" ${feature_dirs} $*"

echo "$cmd"
eval "$cmd"
```

```bash
chmod +x scripts/test-runner.sh
```

- [ ] **Step 5: Write `.gitignore` and `README.md`**

`.gitignore`:
```
/bin/
*.test
/reports/
```

`README.md`:
```markdown
# amd-ci

Ginkgo test framework for the AMD GPU Operator and AMD DRA driver on
OpenShift and Kubernetes. Design: `docs/superpowers/specs/2026-09-25-amd-ci-design.md`.

    make unit-test                          # no cluster needed
    TEST_FEATURES=smoke make run-tests      # needs KUBECONFIG
```

- [ ] **Step 6: Verify the tooling runs**

Run: `make help`
Expected: lists `vet`, `lint`, `verify`, `deps-update`, `unit-test`, `install-ginkgo`, `run-tests`.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "chore: scaffold amd-ci module and tooling

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 2: Config struct and `Load()`

> **Status: ✅ DONE** — commit `750c909`. Spec review passed. Quality review passed with **deferred follow-ups** (not yet fixed): thin env-var round-trip test coverage (~7 of ~26 tagged fields actually verified), no test for `Load()`'s error path, and `clearEnv`'s hand-maintained non-`AMD_`-prefixed var list could drift. Reviewer's suggested fix: one reflection-based test that walks `Config`'s struct tags to both assert every leaf field has a properly-namespaced `envconfig` tag and generate `clearEnv`'s sweep list from the same source — closes all of these at once.

**Files:**
- Create: `internal/config/config.go`, `internal/config/keyvalues.go`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing tests**

`internal/config/config_test.go`:
```go
package config

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

// clearEnv unsets every variable Load reads, restoring them when the test ends.
func clearEnv(t *testing.T) {
	t.Helper()
	extra := map[string]bool{"DRY_RUN": true, "DUMP_FAILED_TESTS": true, "REPORTS_DUMP_DIR": true, "VERBOSE_LEVEL": true}
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(key, "AMD_") || extra[key] {
			t.Setenv(key, "")
			if err := os.Unsetenv(key); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestLoadDefaults(t *testing.T) {
	clearEnv(t)

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if !c.Cleanup || !c.InstallDeps {
		t.Errorf("Cleanup=%v InstallDeps=%v, want both true", c.Cleanup, c.InstallDeps)
	}
	if c.TimeoutScale != 1.0 {
		t.Errorf("TimeoutScale=%v, want 1.0", c.TimeoutScale)
	}
	if c.ReportsDir != "/tmp/reports" {
		t.Errorf("ReportsDir=%q", c.ReportsDir)
	}
	if c.Operator.Source != OperatorSourceRelease || c.Operator.Channel != "alpha" ||
		c.Operator.Catalog != "certified-operators" || c.Operator.Chart != DefaultOperatorChart {
		t.Errorf("Operator defaults wrong: %+v", c.Operator)
	}
	if c.DRA.Source != DRASourceOperator {
		t.Errorf("DRA.Source=%q, want operator", c.DRA.Source)
	}
	// Platform-dependent values stay empty until ApplyPlatformDefaults.
	if c.Platform != "" || c.Namespace != "" || c.Driver.Mode != "" || c.DRA.Chart != "" {
		t.Errorf("platform-dependent fields should be empty: %+v", c)
	}
	if c.Workload.Image != DefaultWorkloadImage || c.Workload.Duration.String() != "1m0s" {
		t.Errorf("Workload defaults wrong: %+v", c.Workload)
	}
}

func TestLoadFromEnv(t *testing.T) {
	clearEnv(t)
	t.Setenv("AMD_PLATFORM", "kubernetes")
	t.Setenv("AMD_CLEANUP", "false")
	t.Setenv("AMD_OPERATOR_SOURCE", "custom")
	t.Setenv("AMD_OPERATOR_BUNDLE", "quay.io/x/bundle:1")
	t.Setenv("AMD_DRIVER_MODE", "preinstalled")
	t.Setenv("AMD_DRA_SOURCE", "helm")
	t.Setenv("AMD_DRA_ARGS", "v=4, feature-gates=A=true")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if c.Platform != PlatformKubernetes || c.Cleanup {
		t.Errorf("Platform=%q Cleanup=%v", c.Platform, c.Cleanup)
	}
	if c.Operator.Source != OperatorSourceCustom || c.Operator.Bundle != "quay.io/x/bundle:1" {
		t.Errorf("Operator=%+v", c.Operator)
	}
	if c.Driver.Mode != DriverModePreinstalled || c.DRA.Source != DRASourceHelm {
		t.Errorf("Driver=%+v DRA=%+v", c.Driver, c.DRA)
	}
	want := KeyValues{"v": "4", "feature-gates": "A=true"}
	if !reflect.DeepEqual(c.DRA.Args, want) {
		t.Errorf("DRA.Args=%v, want %v", c.DRA.Args, want)
	}
}

func TestKeyValuesDecode(t *testing.T) {
	var kv KeyValues
	if err := kv.Decode(""); err != nil || len(kv) != 0 {
		t.Errorf("empty: kv=%v err=%v", kv, err)
	}
	if err := kv.Decode("novalue"); err == nil {
		t.Error("expected error for pair without '='")
	}
	if err := kv.Decode("=v"); err == nil {
		t.Error("expected error for empty key")
	}
}
```

- [ ] **Step 2: Run the tests to confirm they fail**

Run: `go test ./internal/config/`
Expected: FAIL, with compile errors such as `undefined: Load`.

- [ ] **Step 3: Write `internal/config/keyvalues.go`**

```go
package config

import (
	"fmt"
	"strings"
)

// KeyValues is a map read from an env var formatted as "k=v,k=v".
// Only the first '=' in a pair separates key from value, so values may contain '='.
type KeyValues map[string]string

// Decode implements envconfig.Decoder.
func (kv *KeyValues) Decode(value string) error {
	m := KeyValues{}

	if strings.TrimSpace(value) == "" {
		*kv = m

		return nil
	}

	for _, pair := range strings.Split(value, ",") {
		k, v, ok := strings.Cut(pair, "=")
		k = strings.TrimSpace(k)

		if !ok || k == "" {
			return fmt.Errorf("invalid key=value pair %q", pair)
		}

		m[k] = strings.TrimSpace(v)
	}

	*kv = m

	return nil
}
```

- [ ] **Step 4: Write `internal/config/config.go`**

```go
// Package config loads amd-ci settings from environment variables.
package config

import (
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"
)

// Platform is the kind of cluster under test.
type Platform string

// Supported platforms.
const (
	PlatformOpenShift  Platform = "openshift"
	PlatformKubernetes Platform = "kubernetes"
)

// DriverMode selects who provides the amdgpu kernel module.
type DriverMode string

// Supported driver modes.
const (
	DriverModeOperator     DriverMode = "operator"
	DriverModePreinstalled DriverMode = "preinstalled"
)

// OperatorSource selects where the AMD GPU Operator is installed from.
type OperatorSource string

// Supported operator sources.
const (
	OperatorSourceRelease OperatorSource = "release"
	OperatorSourceCustom  OperatorSource = "custom"
)

// DRASource selects how the DRA driver is installed.
type DRASource string

// Supported DRA sources.
const (
	DRASourceOperator DRASource = "operator"
	DRASourceHelm     DRASource = "helm"
)

// Defaults that do not depend on the platform.
const (
	NamespaceOpenShift   = "openshift-amd-gpu"
	NamespaceKubernetes  = "kube-amd-gpu"
	DefaultOperatorChart = "rocm/gpu-operator-charts"
	DefaultDRAChart      = "rocm-k8s-gpu-dra-driver/k8s-gpu-dra-driver"
	DefaultWorkloadImage = "docker.io/rocm/dev-ubuntu-22.04:6.4"
)

// Config holds every amd-ci setting. Fields documented as platform-dependent
// are empty after Load and filled by ApplyPlatformDefaults.
type Config struct {
	Platform        Platform `envconfig:"AMD_PLATFORM"`
	Cleanup         bool     `envconfig:"AMD_CLEANUP" default:"true"`
	Namespace       string   `envconfig:"AMD_NAMESPACE"` // platform-dependent
	TimeoutScale    float64  `envconfig:"AMD_TIMEOUT_SCALE" default:"1.0"`
	InstallDeps     bool     `envconfig:"AMD_INSTALL_DEPS" default:"true"`
	DumpFailedTests bool     `envconfig:"DUMP_FAILED_TESTS" default:"false"`
	ReportsDir      string   `envconfig:"REPORTS_DUMP_DIR" default:"/tmp/reports"`
	DryRun          bool     `envconfig:"DRY_RUN" default:"false"`
	VerboseLevel    string   `envconfig:"VERBOSE_LEVEL" default:"0"`

	Operator OperatorConfig
	Driver   DriverConfig
	DRA      DRAConfig
	Workload WorkloadConfig
}

// OperatorConfig selects the AMD GPU Operator build to install.
type OperatorConfig struct {
	Source  OperatorSource `envconfig:"AMD_OPERATOR_SOURCE" default:"release"`
	Version string         `envconfig:"AMD_OPERATOR_VERSION"`
	Channel string         `envconfig:"AMD_OPERATOR_CHANNEL" default:"alpha"`
	Catalog string         `envconfig:"AMD_OPERATOR_CATALOG" default:"certified-operators"`
	Bundle  string         `envconfig:"AMD_OPERATOR_BUNDLE"`
	Chart   string         `envconfig:"AMD_OPERATOR_CHART" default:"rocm/gpu-operator-charts"`
	Image   string         `envconfig:"AMD_OPERATOR_IMAGE"`
}

// DriverConfig selects the amdgpu kernel driver.
type DriverConfig struct {
	Mode    DriverMode `envconfig:"AMD_DRIVER_MODE"` // platform-dependent
	Version string     `envconfig:"AMD_DRIVER_VERSION"`
	Image   string     `envconfig:"AMD_DRIVER_IMAGE"`
}

// DRAConfig selects the DRA driver build and install method.
type DRAConfig struct {
	Source    DRASource `envconfig:"AMD_DRA_SOURCE" default:"operator"`
	Image     string    `envconfig:"AMD_DRA_IMAGE"`
	Chart     string    `envconfig:"AMD_DRA_CHART"`     // defaulted only when Source is helm
	Namespace string    `envconfig:"AMD_DRA_NAMESPACE"` // defaults to Namespace
	Args      KeyValues `envconfig:"AMD_DRA_ARGS"`
}

// WorkloadConfig selects the ROCm test workload.
type WorkloadConfig struct {
	Image    string        `envconfig:"AMD_WORKLOAD_IMAGE" default:"docker.io/rocm/dev-ubuntu-22.04:6.4"`
	Duration time.Duration `envconfig:"AMD_WORKLOAD_DURATION" default:"60s"`
}

// Load reads Config from the environment. Nested structs are looked up by
// their explicit envconfig tag (envconfig's "Alt" key), so no prefixes apply.
func Load() (*Config, error) {
	var c Config

	if err := envconfig.Process("", &c); err != nil {
		return nil, fmt.Errorf("reading environment: %w", err)
	}

	return &c, nil
}
```

- [ ] **Step 5: Add the dependency and run the tests**

```bash
go get github.com/kelseyhightower/envconfig@v1.4.0
make deps-update
go test ./internal/config/
```
Expected: `ok  	github.com/johnahull/amd-ci/internal/config`

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat(config): load settings from environment

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 3: Platform defaults and scaled timeouts

> **Status: ✅ DONE** — commit `e6d064e`. Spec review passed, quality review passed (only minor doc-comment polish suggestions, not applied).

**Files:**
- Create: `internal/config/defaults.go`
- Test: `internal/config/defaults_test.go`

- [ ] **Step 1: Write the failing tests**

`internal/config/defaults_test.go`:
```go
package config

import (
	"testing"
	"time"
)

func TestApplyPlatformDefaults(t *testing.T) {
	tests := []struct {
		name          string
		in            Config
		platform      Platform
		wantNamespace string
		wantDRANs     string
		wantMode      DriverMode
		wantDRAChart  string
	}{
		{
			name:          "openshift defaults",
			in:            Config{DRA: DRAConfig{Source: DRASourceOperator}},
			platform:      PlatformOpenShift,
			wantNamespace: NamespaceOpenShift,
			wantDRANs:     NamespaceOpenShift,
			wantMode:      DriverModeOperator,
		},
		{
			name:          "kubernetes defaults with helm DRA",
			in:            Config{DRA: DRAConfig{Source: DRASourceHelm}},
			platform:      PlatformKubernetes,
			wantNamespace: NamespaceKubernetes,
			wantDRANs:     NamespaceKubernetes,
			wantMode:      DriverModePreinstalled,
			wantDRAChart:  DefaultDRAChart,
		},
		{
			name: "explicit values win",
			in: Config{
				Namespace: "gpu",
				Driver:    DriverConfig{Mode: DriverModeOperator},
				DRA:       DRAConfig{Source: DRASourceHelm, Chart: "/src/chart", Namespace: "dra"},
			},
			platform:      PlatformKubernetes,
			wantNamespace: "gpu",
			wantDRANs:     "dra",
			wantMode:      DriverModeOperator,
			wantDRAChart:  "/src/chart",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := tt.in
			c.ApplyPlatformDefaults(tt.platform)

			if c.Platform != tt.platform {
				t.Errorf("Platform=%q, want %q", c.Platform, tt.platform)
			}
			if c.Namespace != tt.wantNamespace || c.DRA.Namespace != tt.wantDRANs {
				t.Errorf("Namespace=%q DRA.Namespace=%q, want %q %q", c.Namespace, c.DRA.Namespace, tt.wantNamespace, tt.wantDRANs)
			}
			if c.Driver.Mode != tt.wantMode {
				t.Errorf("Driver.Mode=%q, want %q", c.Driver.Mode, tt.wantMode)
			}
			if c.DRA.Chart != tt.wantDRAChart {
				t.Errorf("DRA.Chart=%q, want %q", c.DRA.Chart, tt.wantDRAChart)
			}
		})
	}
}

func TestTimeout(t *testing.T) {
	c := Config{TimeoutScale: 2.5}
	if got := c.Timeout(10 * time.Minute); got != 25*time.Minute {
		t.Errorf("Timeout=%v, want 25m", got)
	}
}
```

- [ ] **Step 2: Run the tests to confirm they fail**

Run: `go test ./internal/config/`
Expected: FAIL, with `c.ApplyPlatformDefaults undefined`.

- [ ] **Step 3: Write `internal/config/defaults.go`**

```go
package config

import "time"

// ApplyPlatformDefaults records the detected platform and fills every
// platform-dependent field the user left unset.
func (c *Config) ApplyPlatformDefaults(p Platform) {
	c.Platform = p

	if c.Namespace == "" {
		c.Namespace = NamespaceKubernetes
		if p == PlatformOpenShift {
			c.Namespace = NamespaceOpenShift
		}
	}

	// RHCOS nodes are immutable, so OpenShift relies on KMM to build the driver;
	// Kubernetes lab nodes usually have amdgpu installed on the host.
	if c.Driver.Mode == "" {
		c.Driver.Mode = DriverModePreinstalled
		if p == PlatformOpenShift {
			c.Driver.Mode = DriverModeOperator
		}
	}

	if c.DRA.Namespace == "" {
		c.DRA.Namespace = c.Namespace
	}

	if c.DRA.Source == DRASourceHelm && c.DRA.Chart == "" {
		c.DRA.Chart = DefaultDRAChart
	}
}

// Timeout scales a default timeout by AMD_TIMEOUT_SCALE.
func (c *Config) Timeout(d time.Duration) time.Duration {
	return time.Duration(float64(d) * c.TimeoutScale)
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/config/`
Expected: `ok`

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat(config): apply platform-dependent defaults

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 4: Config validation

> **Status: ✅ DONE** — commit `170b904`, fix commit `c7688f2` (added a missing test case: `DRA.Source=helm` + `Chart` set is a valid combination that was never asserted). Spec + quality reviews passed after the fix.

**Files:**
- Create: `internal/config/validate.go`
- Test: `internal/config/validate_test.go`

- [ ] **Step 1: Write the failing tests**

`internal/config/validate_test.go`:
```go
package config

import (
	"strings"
	"testing"
)

// validConfig returns a config that passes Validate on the given platform.
func validConfig(p Platform) Config {
	c := Config{
		TimeoutScale: 1,
		Operator:     OperatorConfig{Source: OperatorSourceRelease, Chart: DefaultOperatorChart},
		DRA:          DRAConfig{Source: DRASourceOperator},
	}
	c.ApplyPlatformDefaults(p)

	return c
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(c *Config)
		p       Platform
		wantErr string // empty means valid
	}{
		{name: "openshift defaults valid", p: PlatformOpenShift, mutate: func(*Config) {}},
		{name: "kubernetes defaults valid", p: PlatformKubernetes, mutate: func(*Config) {}},
		{
			name: "unknown platform", p: PlatformKubernetes,
			mutate:  func(c *Config) { c.Platform = "rancher" },
			wantErr: "AMD_PLATFORM",
		},
		{
			name: "unknown operator source", p: PlatformKubernetes,
			mutate:  func(c *Config) { c.Operator.Source = "nightly" },
			wantErr: "AMD_OPERATOR_SOURCE",
		},
		{
			name: "unknown driver mode", p: PlatformKubernetes,
			mutate:  func(c *Config) { c.Driver.Mode = "dkms" },
			wantErr: "AMD_DRIVER_MODE",
		},
		{
			name: "unknown DRA source", p: PlatformKubernetes,
			mutate:  func(c *Config) { c.DRA.Source = "kustomize" },
			wantErr: "AMD_DRA_SOURCE",
		},
		{
			name: "non-positive timeout scale", p: PlatformKubernetes,
			mutate:  func(c *Config) { c.TimeoutScale = 0 },
			wantErr: "AMD_TIMEOUT_SCALE",
		},
		{
			name: "custom operator on openshift needs bundle", p: PlatformOpenShift,
			mutate:  func(c *Config) { c.Operator.Source = OperatorSourceCustom },
			wantErr: "AMD_OPERATOR_BUNDLE",
		},
		{
			name: "custom operator on openshift with bundle", p: PlatformOpenShift,
			mutate: func(c *Config) {
				c.Operator.Source = OperatorSourceCustom
				c.Operator.Bundle = "quay.io/x/bundle:1"
			},
		},
		{
			name: "custom operator on kubernetes needs chart or image", p: PlatformKubernetes,
			mutate:  func(c *Config) { c.Operator.Source = OperatorSourceCustom },
			wantErr: "AMD_OPERATOR_CHART",
		},
		{
			name: "custom operator on kubernetes with local chart", p: PlatformKubernetes,
			mutate: func(c *Config) {
				c.Operator.Source = OperatorSourceCustom
				c.Operator.Chart = "/src/gpu-operator/helm-charts-k8s"
			},
		},
		{
			name: "custom operator on kubernetes with image only", p: PlatformKubernetes,
			mutate: func(c *Config) {
				c.Operator.Source = OperatorSourceCustom
				c.Operator.Image = "quay.io/x/operator:dev"
			},
		},
		{
			name: "bundle on kubernetes", p: PlatformKubernetes,
			mutate:  func(c *Config) { c.Operator.Bundle = "quay.io/x/bundle:1" },
			wantErr: "only supported on OpenShift",
		},
		{
			name: "DRA chart with operator source", p: PlatformKubernetes,
			mutate:  func(c *Config) { c.DRA.Chart = "/src/chart" },
			wantErr: "AMD_DRA_CHART requires AMD_DRA_SOURCE=helm",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validConfig(tt.p)
			tt.mutate(&c)
			err := c.Validate()

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				return
			}

			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateReportsAllErrors(t *testing.T) {
	c := validConfig(PlatformKubernetes)
	c.TimeoutScale = -1
	c.DRA.Source = "bogus"

	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "AMD_TIMEOUT_SCALE") || !strings.Contains(err.Error(), "AMD_DRA_SOURCE") {
		t.Fatalf("want both errors reported, got %v", err)
	}
}
```

- [ ] **Step 2: Run the tests to confirm they fail**

Run: `go test ./internal/config/`
Expected: FAIL, with `c.Validate undefined`.

- [ ] **Step 3: Write `internal/config/validate.go`**

```go
package config

import (
	"errors"
	"fmt"
)

// Validate rejects unknown enum values and invalid combinations. It must run
// after ApplyPlatformDefaults. All problems are reported together.
func (c *Config) Validate() error {
	var errs []error

	switch c.Platform {
	case PlatformOpenShift, PlatformKubernetes:
	default:
		errs = append(errs, fmt.Errorf("AMD_PLATFORM must be %q or %q, got %q", PlatformOpenShift, PlatformKubernetes, c.Platform))
	}

	switch c.Operator.Source {
	case OperatorSourceRelease, OperatorSourceCustom:
	default:
		errs = append(errs, fmt.Errorf("AMD_OPERATOR_SOURCE must be %q or %q, got %q", OperatorSourceRelease, OperatorSourceCustom, c.Operator.Source))
	}

	switch c.Driver.Mode {
	case DriverModeOperator, DriverModePreinstalled:
	default:
		errs = append(errs, fmt.Errorf("AMD_DRIVER_MODE must be %q or %q, got %q", DriverModeOperator, DriverModePreinstalled, c.Driver.Mode))
	}

	switch c.DRA.Source {
	case DRASourceOperator, DRASourceHelm:
	default:
		errs = append(errs, fmt.Errorf("AMD_DRA_SOURCE must be %q or %q, got %q", DRASourceOperator, DRASourceHelm, c.DRA.Source))
	}

	if c.TimeoutScale <= 0 {
		errs = append(errs, fmt.Errorf("AMD_TIMEOUT_SCALE must be > 0, got %v", c.TimeoutScale))
	}

	if c.Operator.Source == OperatorSourceCustom {
		switch c.Platform {
		case PlatformOpenShift:
			if c.Operator.Bundle == "" {
				errs = append(errs, errors.New("AMD_OPERATOR_SOURCE=custom on OpenShift requires AMD_OPERATOR_BUNDLE"))
			}
		case PlatformKubernetes:
			if c.Operator.Chart == DefaultOperatorChart && c.Operator.Image == "" {
				errs = append(errs, errors.New("AMD_OPERATOR_SOURCE=custom on Kubernetes requires a non-default AMD_OPERATOR_CHART or AMD_OPERATOR_IMAGE"))
			}
		}
	}

	if c.Platform == PlatformKubernetes && c.Operator.Bundle != "" {
		errs = append(errs, errors.New("AMD_OPERATOR_BUNDLE is only supported on OpenShift; use AMD_OPERATOR_CHART on Kubernetes"))
	}

	if c.DRA.Source == DRASourceOperator && c.DRA.Chart != "" {
		errs = append(errs, errors.New("AMD_DRA_CHART requires AMD_DRA_SOURCE=helm"))
	}

	return errors.Join(errs...)
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/config/`
Expected: `ok`

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat(config): validate settings and combinations

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 5: Platform detection

> **Status: ✅ DONE** — commit `3e2d62d`, fix commit `ceea651` (added a test for the `ServerGroups()` failure path, which was previously unexercised). Spec + quality reviews passed after the fix.
>
> **Deferred follow-up** (not yet fixed): `internal/platform.Detect` and `internal/config.Config.Validate` each independently validate the `Platform` enum, with two different error message formats. This will drift if only one is updated later. Reviewer's suggested fix: have `Detect` delegate to a shared validation helper exported from `internal/config`, rather than duplicating the enum switch — but note `Detect`'s own test (`"invalid override"`, `wantErr: true`) requires `Detect` to keep *some* form of validation, so this isn't a simple "delete the check" fix; it needs a real design decision.

**Files:**
- Create: `internal/platform/detect.go`
- Test: `internal/platform/detect_test.go`

- [ ] **Step 1: Write the failing tests**

`internal/platform/detect_test.go`:
```go
package platform

import (
	"testing"

	"github.com/johnahull/amd-ci/internal/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakediscovery "k8s.io/client-go/discovery/fake"
	clienttesting "k8s.io/client-go/testing"
)

func fakeDiscovery(groupVersions ...string) *fakediscovery.FakeDiscovery {
	var resources []*metav1.APIResourceList
	for _, gv := range groupVersions {
		resources = append(resources, &metav1.APIResourceList{GroupVersion: gv})
	}

	return &fakediscovery.FakeDiscovery{Fake: &clienttesting.Fake{Resources: resources}}
}

func TestDetect(t *testing.T) {
	tests := []struct {
		name     string
		groups   []string
		override config.Platform
		want     config.Platform
		wantErr  bool
	}{
		{name: "openshift by API group", groups: []string{"v1", "apps/v1", "config.openshift.io/v1"}, want: config.PlatformOpenShift},
		{name: "kubernetes by absence", groups: []string{"v1", "apps/v1"}, want: config.PlatformKubernetes},
		{name: "override wins", groups: []string{"config.openshift.io/v1"}, override: config.PlatformKubernetes, want: config.PlatformKubernetes},
		{name: "invalid override", override: "rancher", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Detect(fakeDiscovery(tt.groups...), tt.override)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err=%v, wantErr=%v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run the tests to confirm they fail**

```bash
go get k8s.io/api@v0.35.1 k8s.io/apimachinery@v0.35.1 k8s.io/client-go@v0.35.1
go test ./internal/platform/
```
Expected: FAIL, with `undefined: Detect`.

- [ ] **Step 3: Write `internal/platform/detect.go`**

```go
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
```

- [ ] **Step 4: Vendor and run the tests**

```bash
make deps-update
go test ./internal/platform/
```
Expected: `ok`

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat(platform): detect OpenShift vs Kubernetes

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 6: Minimal `DeviceConfig` API types

> **Status: ✅ DONE** — commit `4cec9c3`, fix commit `478a449` (extended the JSON-tag regression test to also cover `DeviceConfigStatus` fields and `Spec.Selector`, including `DeviceConfigStatus.Drivers`'s intentional Go/JSON name mismatch — Go field `Drivers` (plural) is tagged `json:"driver"` (singular) to match upstream; the fix locks this in with a test verified via mutation testing to actually catch a regression). Spec + quality reviews passed after the fix.
>
> Note: `k8s.io/utils` was pinned in this task's Step 5 to `v0.0.0-20260108192941-914a6e750570`, but the implementer used whatever version was already vendored transitively from Task 5's `k8s.io/client-go` (`v0.0.0-20251002143259-bc988d571ff4`) instead, since it already provides `ptr.To` and avoids a version conflict. Reviewed and accepted as a reasonable, low-risk judgment call.

The types are copied (trimmed) from `~/redhat/amd/gpu-operator/api/v1alpha1/deviceconfig_types.go` instead of importing that Go module. The upstream module needs Go 1.26.7 and k8s.io v0.36, and pulls in prometheus-operator. Only the fields this framework reads or sets are modeled. JSON names must match upstream **exactly**. For example, the device plugin toggle is `enableDevicePlugin`, not `enable`.

**Files:**
- Create: `pkg/amdgpu/v1alpha1/types.go`, `pkg/amdgpu/v1alpha1/register.go`
- Test: `pkg/amdgpu/v1alpha1/types_test.go`

- [ ] **Step 1: Write the failing tests**

`pkg/amdgpu/v1alpha1/types_test.go`:
```go
package v1alpha1

import (
	"encoding/json"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
)

func TestJSONFieldNamesMatchUpstream(t *testing.T) {
	dc := DeviceConfig{Spec: DeviceConfigSpec{
		Driver:       DriverSpec{Enable: ptr.To(false)},
		DevicePlugin: DevicePluginSpec{EnableDevicePlugin: ptr.To(false)},
		DRADriver:    DRADriverSpec{Enable: ptr.To(true), Image: "img", CmdLineArguments: map[string]string{"v": "4"}},
	}}

	b, err := json.Marshal(dc)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		`"driver":{"enable":false}`,
		`"devicePlugin":{"enableDevicePlugin":false}`,
		`"draDriver":{"enable":true,"image":"img","cmdLineArguments":{"v":"4"}}`,
	} {
		if !strings.Contains(string(b), want) {
			t.Errorf("JSON %s missing %s", b, want)
		}
	}
}

func TestSchemeRegistration(t *testing.T) {
	s := runtime.NewScheme()
	if err := AddToScheme(s); err != nil {
		t.Fatal(err)
	}

	gvks, _, err := s.ObjectKinds(&DeviceConfig{})
	if err != nil || len(gvks) != 1 || gvks[0].Kind != "DeviceConfig" || gvks[0].Group != "amd.com" {
		t.Fatalf("gvks=%v err=%v", gvks, err)
	}
}

func TestDeepCopyIsIndependent(t *testing.T) {
	orig := &DeviceConfig{Spec: DeviceConfigSpec{DRADriver: DRADriverSpec{CmdLineArguments: map[string]string{"v": "4"}}}}
	cp := orig.DeepCopy()
	cp.Spec.DRADriver.CmdLineArguments["v"] = "9"

	if orig.Spec.DRADriver.CmdLineArguments["v"] != "4" {
		t.Error("DeepCopy shares the map with the original")
	}
}
```

- [ ] **Step 2: Run the tests to confirm they fail**

Run: `go test ./pkg/amdgpu/v1alpha1/`
Expected: FAIL, with `undefined: DeviceConfig`.

- [ ] **Step 3: Write `pkg/amdgpu/v1alpha1/types.go`**

```go
// Package v1alpha1 is a trimmed copy of the AMD GPU Operator amd.com/v1alpha1
// API (github.com/ROCm/gpu-operator/api/v1alpha1). Only fields amd-ci reads or
// sets are modeled; updates must go through merge patches (see pkg/amdgpu) so
// unmodeled fields on the server are preserved.
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DeviceConfig is the AMD GPU Operator's top-level custom resource.
type DeviceConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DeviceConfigSpec   `json:"spec,omitempty"`
	Status DeviceConfigStatus `json:"status,omitempty"`
}

// DeviceConfigList is a list of DeviceConfig.
type DeviceConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []DeviceConfig `json:"items"`
}

// DeviceConfigSpec is the subset of the upstream spec used by amd-ci.
type DeviceConfigSpec struct {
	Driver       DriverSpec        `json:"driver,omitempty"`
	DevicePlugin DevicePluginSpec  `json:"devicePlugin,omitempty"`
	DRADriver    DRADriverSpec     `json:"draDriver,omitempty"`
	Selector     map[string]string `json:"selector,omitempty"`
}

// DriverSpec controls the amdgpu kernel module (built/loaded through KMM).
type DriverSpec struct {
	Enable  *bool  `json:"enable,omitempty"`
	Version string `json:"version,omitempty"`
	Image   string `json:"image,omitempty"`
}

// DevicePluginSpec controls the amd.com/gpu device plugin.
type DevicePluginSpec struct {
	EnableDevicePlugin *bool  `json:"enableDevicePlugin,omitempty"`
	DevicePluginImage  string `json:"devicePluginImage,omitempty"`
}

// DRADriverSpec controls the operator-managed DRA driver. The operator rejects
// enabling it together with the device plugin.
type DRADriverSpec struct {
	Enable           *bool             `json:"enable,omitempty"`
	Image            string            `json:"image,omitempty"`
	ImagePullPolicy  string            `json:"imagePullPolicy,omitempty"`
	CmdLineArguments map[string]string `json:"cmdLineArguments,omitempty"`
}

// DeviceConfigStatus is the subset of the upstream status used by amd-ci.
type DeviceConfigStatus struct {
	DevicePlugin     DeploymentStatus        `json:"devicePlugin,omitempty"`
	Drivers          DeploymentStatus        `json:"driver,omitempty"`
	NodeModuleStatus map[string]ModuleStatus `json:"nodeModuleStatus,omitempty"`
	Conditions       []metav1.Condition      `json:"conditions,omitempty"`
}

// DeploymentStatus reports how many nodes run a component.
type DeploymentStatus struct {
	NodesMatchingSelectorNumber int32 `json:"nodesMatchingSelectorNumber,omitempty"`
	DesiredNumber               int32 `json:"desiredNumber,omitempty"`
	AvailableNumber             int32 `json:"availableNumber,omitempty"`
}

// ModuleStatus reports the kernel module state on one node.
type ModuleStatus struct {
	ContainerImage string `json:"containerImage,omitempty"`
	KernelVersion  string `json:"kernelVersion,omitempty"`
	Status         string `json:"status,omitempty"`
}
```

- [ ] **Step 4: Write `pkg/amdgpu/v1alpha1/register.go`**

```go
package v1alpha1

import (
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	// GroupVersion is the API group and version of DeviceConfig.
	GroupVersion = schema.GroupVersion{Group: "amd.com", Version: "v1alpha1"}

	// SchemeBuilder registers this package's types.
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)

	// AddToScheme adds this package's types to a scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)

func addKnownTypes(s *runtime.Scheme) error {
	s.AddKnownTypes(GroupVersion, &DeviceConfig{}, &DeviceConfigList{})
	metav1.AddToGroupVersion(s, GroupVersion)

	return nil
}

// DeepCopy returns an independent copy. A JSON round trip is used instead of
// generated deepcopy code; these objects are small and not on a hot path.
func (in *DeviceConfig) DeepCopy() *DeviceConfig {
	out := &DeviceConfig{}
	jsonCopy(in, out)

	return out
}

// DeepCopyObject implements runtime.Object.
func (in *DeviceConfig) DeepCopyObject() runtime.Object { return in.DeepCopy() }

// DeepCopyObject implements runtime.Object.
func (in *DeviceConfigList) DeepCopyObject() runtime.Object {
	out := &DeviceConfigList{}
	jsonCopy(in, out)

	return out
}

func jsonCopy(in, out any) {
	b, err := json.Marshal(in)
	if err != nil {
		panic(fmt.Sprintf("deepcopy marshal: %v", err))
	}

	if err := json.Unmarshal(b, out); err != nil {
		panic(fmt.Sprintf("deepcopy unmarshal: %v", err))
	}
}
```

- [ ] **Step 5: Vendor and run the tests**

```bash
go get k8s.io/utils@v0.0.0-20260108192941-914a6e750570
make deps-update
go test ./pkg/amdgpu/v1alpha1/
```
Expected: `ok`

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat(amdgpu): add minimal amd.com/v1alpha1 DeviceConfig types

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 7: API clients

> **Status: ✅ DONE** — commit `3726bf8`. Spec + quality reviews passed with no issues (verified against vendored source that the kubeconfig fallback chain, `Settings.Discovery`'s compatibility with `platform.Detect`, and non-blocking client construction all behave as documented).

**Files:**
- Create: `pkg/clients/clients.go`
- Test: `pkg/clients/clients_test.go`

- [ ] **Step 1: Write the failing test**

`pkg/clients/clients_test.go`:
```go
package clients

import (
	"testing"

	amdv1alpha1 "github.com/johnahull/amd-ci/pkg/amdgpu/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestNewSchemeRegistersTypes(t *testing.T) {
	s, err := NewScheme()
	if err != nil {
		t.Fatal(err)
	}

	for _, obj := range []runtime.Object{&corev1.Node{}, &resourcev1.ResourceSlice{}, &amdv1alpha1.DeviceConfig{}} {
		if _, _, err := s.ObjectKinds(obj); err != nil {
			t.Errorf("%T not registered: %v", obj, err)
		}
	}
}
```

- [ ] **Step 2: Run the test to confirm it fails**

Run: `go test ./pkg/clients/`
Expected: FAIL, with `undefined: NewScheme`.

- [ ] **Step 3: Write `pkg/clients/clients.go`**

```go
// Package clients builds the Kubernetes clients shared by every suite.
package clients

import (
	"fmt"
	"os"

	amdv1alpha1 "github.com/johnahull/amd-ci/pkg/amdgpu/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Settings bundles the clients for one cluster. The embedded controller-runtime
// client is the default way to read and write objects. OpenShift-only schemes
// are added by internal/platform (Plan 2), never required here.
type Settings struct {
	client.Client

	Config    *rest.Config
	K8s       kubernetes.Interface
	Discovery discovery.DiscoveryInterface
}

// NewScheme returns a scheme with the core Kubernetes types (including
// resource.k8s.io/v1) and amd.com/v1alpha1.
func NewScheme() (*runtime.Scheme, error) {
	s := runtime.NewScheme()

	if err := clientgoscheme.AddToScheme(s); err != nil {
		return nil, fmt.Errorf("adding client-go scheme: %w", err)
	}

	if err := amdv1alpha1.AddToScheme(s); err != nil {
		return nil, fmt.Errorf("adding amd.com/v1alpha1 scheme: %w", err)
	}

	return s, nil
}

// New connects using kubeconfig, falling back to $KUBECONFIG and then to
// in-cluster config.
func New(kubeconfig string) (*Settings, error) {
	if kubeconfig == "" {
		kubeconfig = os.Getenv("KUBECONFIG")
	}

	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("loading kubeconfig %q: %w", kubeconfig, err)
	}

	k8s, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating clientset: %w", err)
	}

	scheme, err := NewScheme()
	if err != nil {
		return nil, err
	}

	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("creating controller-runtime client: %w", err)
	}

	return &Settings{Client: c, Config: cfg, K8s: k8s, Discovery: k8s.Discovery()}, nil
}
```

- [ ] **Step 4: Vendor and run the test**

```bash
go get sigs.k8s.io/controller-runtime@v0.23.1
make deps-update
go test ./pkg/clients/
```
Expected: `ok`

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat(clients): add shared cluster clients and scheme

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 8: `DeviceConfig` builder

> **Status: ✅ DONE** — commit `fd77b79`. Spec + quality reviews passed with no blocking issues.
>
> **Deferred follow-up / design decision needed before Plan 2** (not yet fixed): the `With*` methods on `Builder` treat an empty string / nil-or-empty map as "leave unchanged," so there is currently no way to explicitly clear `Driver.Version`, `Driver.Image`, `DRADriver.Image`, or `DRADriver.CmdLineArguments` back to empty through the public API — a caller needing that must mutate `Definition` directly, which bypasses the merge-patch diffing design. This doesn't block Plan 1 (Tasks 9-10 don't need it), but Plan 2's deploy suite likely will. Decide either to (a) accept this as a documented limitation, or (b) add an explicit-clear escape hatch (e.g. a sentinel value or dedicated `ClearX()` methods) before Plan 2 depends on update flows.

**Files:**
- Create: `pkg/amdgpu/builder.go`
- Test: `pkg/amdgpu/builder_test.go`

- [ ] **Step 1: Write the failing tests**

`pkg/amdgpu/builder_test.go`:
```go
package amdgpu

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/johnahull/amd-ci/pkg/clients"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	testName = "test-deviceconfig"
	testNs   = "kube-amd-gpu"
)

func newFakeClient(t *testing.T) client.Client {
	t.Helper()

	s, err := clients.NewScheme()
	if err != nil {
		t.Fatal(err)
	}

	return fake.NewClientBuilder().WithScheme(s).Build()
}

func TestCreateThenPull(t *testing.T) {
	ctx := context.Background()
	c := newFakeClient(t)

	err := NewBuilder(c, testName, testNs).
		WithSelector(map[string]string{"feature.node.kubernetes.io/amd-gpu": "true"}).
		WithDriver(false, "", "").
		WithDevicePlugin(true).
		Create(ctx)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	b, err := Pull(ctx, c, testName, testNs)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}

	spec := b.Definition.Spec
	if *spec.Driver.Enable || !*spec.DevicePlugin.EnableDevicePlugin || spec.Selector["feature.node.kubernetes.io/amd-gpu"] != "true" {
		t.Errorf("unexpected spec: %+v", spec)
	}
}

func TestApplySendsOnlyChangedFields(t *testing.T) {
	ctx := context.Background()
	c := newFakeClient(t)

	if err := NewBuilder(c, testName, testNs).WithDriver(true, "6.4.1", "").WithDevicePlugin(true).Create(ctx); err != nil {
		t.Fatal(err)
	}

	b, err := Pull(ctx, c, testName, testNs)
	if err != nil {
		t.Fatal(err)
	}

	b.WithDevicePlugin(false).WithDRADriver(true, "quay.io/x/dra:dev", map[string]string{"v": "4"})

	raw, err := client.MergeFrom(b.original).Data(b.Definition)
	if err != nil {
		t.Fatal(err)
	}

	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}

	want := map[string]any{"spec": map[string]any{
		"devicePlugin": map[string]any{"enableDevicePlugin": false},
		"draDriver":    map[string]any{"enable": true, "image": "quay.io/x/dra:dev", "cmdLineArguments": map[string]any{"v": "4"}},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("patch = %s, want only devicePlugin+draDriver changes", raw)
	}

	if err := b.Apply(ctx); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	after, err := Pull(ctx, c, testName, testNs)
	if err != nil {
		t.Fatal(err)
	}

	if *after.Definition.Spec.DevicePlugin.EnableDevicePlugin || !*after.Definition.Spec.DRADriver.Enable ||
		after.Definition.Spec.Driver.Version != "6.4.1" {
		t.Errorf("unexpected spec after Apply: %+v", after.Definition.Spec)
	}
}

func TestApplyRequiresPullOrCreate(t *testing.T) {
	if err := NewBuilder(newFakeClient(t), testName, testNs).Apply(context.Background()); err == nil {
		t.Fatal("expected error applying a builder that was never created or pulled")
	}
}

func TestExistsAndDelete(t *testing.T) {
	ctx := context.Background()
	c := newFakeClient(t)
	b := NewBuilder(c, testName, testNs)

	if ok, err := b.Exists(ctx); ok || err != nil {
		t.Fatalf("Exists before create = %v, %v", ok, err)
	}

	if err := b.Create(ctx); err != nil {
		t.Fatal(err)
	}

	if ok, err := b.Exists(ctx); !ok || err != nil {
		t.Fatalf("Exists after create = %v, %v", ok, err)
	}

	if err := b.Delete(ctx); err != nil {
		t.Fatal(err)
	}

	if err := b.Delete(ctx); err != nil {
		t.Fatalf("second Delete should ignore NotFound: %v", err)
	}
}
```

- [ ] **Step 2: Run the tests to confirm they fail**

Run: `go test ./pkg/amdgpu/`
Expected: FAIL, with `undefined: NewBuilder`.

- [ ] **Step 3: Write `pkg/amdgpu/builder.go`**

```go
// Package amdgpu builds and manages the AMD GPU Operator DeviceConfig.
package amdgpu

import (
	"context"
	"fmt"

	amdv1alpha1 "github.com/johnahull/amd-ci/pkg/amdgpu/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Builder wraps one DeviceConfig. Edit Definition through the With* methods,
// then Create (new object) or Apply (existing object).
type Builder struct {
	client     client.Client
	Definition *amdv1alpha1.DeviceConfig
	// original is the last state read from or written to the server; Apply
	// diffs against it so only changed fields are sent.
	original *amdv1alpha1.DeviceConfig
}

// NewBuilder starts a DeviceConfig that does not exist on the server yet.
func NewBuilder(c client.Client, name, namespace string) *Builder {
	return &Builder{
		client: c,
		Definition: &amdv1alpha1.DeviceConfig{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		},
	}
}

// Pull loads an existing DeviceConfig.
func Pull(ctx context.Context, c client.Client, name, namespace string) (*Builder, error) {
	dc := &amdv1alpha1.DeviceConfig{}
	if err := c.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, dc); err != nil {
		return nil, fmt.Errorf("getting DeviceConfig %s/%s: %w", namespace, name, err)
	}

	return &Builder{client: c, Definition: dc, original: dc.DeepCopy()}, nil
}

// WithSelector sets the node selector the operator uses to pick GPU nodes.
func (b *Builder) WithSelector(selector map[string]string) *Builder {
	b.Definition.Spec.Selector = selector

	return b
}

// WithDriver enables or disables operator-managed (KMM) driver install.
// Empty version or image leaves the current value unchanged.
func (b *Builder) WithDriver(enable bool, version, image string) *Builder {
	b.Definition.Spec.Driver.Enable = ptr.To(enable)

	if version != "" {
		b.Definition.Spec.Driver.Version = version
	}

	if image != "" {
		b.Definition.Spec.Driver.Image = image
	}

	return b
}

// WithDevicePlugin enables or disables the amd.com/gpu device plugin.
func (b *Builder) WithDevicePlugin(enable bool) *Builder {
	b.Definition.Spec.DevicePlugin.EnableDevicePlugin = ptr.To(enable)

	return b
}

// WithDRADriver enables or disables the operator-managed DRA driver. Empty
// image or args leave the current values unchanged.
func (b *Builder) WithDRADriver(enable bool, image string, args map[string]string) *Builder {
	b.Definition.Spec.DRADriver.Enable = ptr.To(enable)

	if image != "" {
		b.Definition.Spec.DRADriver.Image = image
	}

	if len(args) > 0 {
		b.Definition.Spec.DRADriver.CmdLineArguments = args
	}

	return b
}

// Exists reports whether the DeviceConfig is on the server.
func (b *Builder) Exists(ctx context.Context) (bool, error) {
	err := b.client.Get(ctx, client.ObjectKeyFromObject(b.Definition), &amdv1alpha1.DeviceConfig{})
	if apierrors.IsNotFound(err) {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("getting DeviceConfig %s: %w", b.key(), err)
	}

	return true, nil
}

// Create creates the DeviceConfig.
func (b *Builder) Create(ctx context.Context) error {
	if err := b.client.Create(ctx, b.Definition); err != nil {
		return fmt.Errorf("creating DeviceConfig %s: %w", b.key(), err)
	}

	b.original = b.Definition.DeepCopy()

	return nil
}

// Apply sends the fields changed since Pull/Create as a JSON merge patch, so
// spec fields this package does not model are left untouched on the server.
func (b *Builder) Apply(ctx context.Context) error {
	if b.original == nil {
		return fmt.Errorf("DeviceConfig %s: Apply requires Pull or Create first", b.key())
	}

	if err := b.client.Patch(ctx, b.Definition, client.MergeFrom(b.original)); err != nil {
		return fmt.Errorf("patching DeviceConfig %s: %w", b.key(), err)
	}

	b.original = b.Definition.DeepCopy()

	return nil
}

// Delete deletes the DeviceConfig; NotFound is not an error.
func (b *Builder) Delete(ctx context.Context) error {
	if err := b.client.Delete(ctx, b.Definition); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("deleting DeviceConfig %s: %w", b.key(), err)
	}

	return nil
}

func (b *Builder) key() string {
	return b.Definition.Namespace + "/" + b.Definition.Name
}
```

- [ ] **Step 4: Vendor and run the tests**

```bash
make deps-update
go test ./pkg/amdgpu/...
```
Expected: `ok` for both `pkg/amdgpu` and `pkg/amdgpu/v1alpha1`.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat(amdgpu): add DeviceConfig builder with merge-patch updates

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 9: GPU discovery (nodes and ResourceSlices)

> **Status: ✅ DONE** — commit `e0bb5f4`. `go test ./...` and `make verify` passed.

Node labels checked, in order:
1. `feature.node.kubernetes.io/amd-gpu=true` / `amd-vgpu=true`, from the NodeFeatureRule in AMD's docs, which the operator's default selector uses.
2. Raw NFD PCI labels for vendor `1002`, which exist before any AMD-specific rule is installed: `pci-<class>_1002.present` (NFD default fields) or `pci-1002_<device>.present` (AMD's OpenShift NFD config uses `vendor,device`).

The DRA driver publishes an unqualified `type` attribute. Full GPUs have `type=amdgpu`; partitions and VFIO devices use other values and are not counted. Slices from an older generation of a pool are ignored, since they are stale.

**Files:**
- Create: `internal/discovery/nodes.go`, `internal/discovery/slices.go`
- Test: `internal/discovery/nodes_test.go`, `internal/discovery/slices_test.go`

- [ ] **Step 1: Write the failing tests**

`internal/discovery/nodes_test.go`:
```go
package discovery

import (
	"context"
	"testing"

	"github.com/johnahull/amd-ci/pkg/clients"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newFakeClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()

	s, err := clients.NewScheme()
	if err != nil {
		t.Fatal(err)
	}

	return fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()
}

func TestIsAMDGPUNode(t *testing.T) {
	tests := []struct {
		labels map[string]string
		want   bool
	}{
		{map[string]string{LabelAMDGPU: "true"}, true},
		{map[string]string{LabelAMDVGPU: "true"}, true},
		{map[string]string{LabelAMDGPU: "false"}, false},
		{map[string]string{"feature.node.kubernetes.io/pci-0380_1002.present": "true"}, true},
		{map[string]string{"feature.node.kubernetes.io/pci-1002_74a1.present": "true"}, true},
		{map[string]string{"feature.node.kubernetes.io/pci-0300_10de.present": "true"}, false}, // NVIDIA
		{map[string]string{"feature.node.kubernetes.io/pci-0300_1a03.present": "true"}, false}, // ASPEED BMC
		{map[string]string{"node-role.kubernetes.io/worker": ""}, false},
	}

	for _, tt := range tests {
		if got := IsAMDGPUNode(tt.labels); got != tt.want {
			t.Errorf("IsAMDGPUNode(%v) = %v, want %v", tt.labels, got, tt.want)
		}
	}
}

func TestGPUNodes(t *testing.T) {
	c := newFakeClient(t,
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "gpu-1", Labels: map[string]string{LabelAMDGPU: "true"}}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "cpu-1"}},
	)

	nodes, err := GPUNodes(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}

	if len(nodes) != 1 || nodes[0].Name != "gpu-1" {
		t.Fatalf("GPUNodes = %v, want [gpu-1]", nodes)
	}
}
```

`internal/discovery/slices_test.go`:
```go
package discovery

import (
	"context"
	"reflect"
	"testing"

	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func device(name, typ string) resourcev1.Device {
	return resourcev1.Device{
		Name:       name,
		Attributes: map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{"type": {StringValue: ptr.To(typ)}},
	}
}

func slice(name, driver, node, pool string, generation int64, devices ...resourcev1.Device) *resourcev1.ResourceSlice {
	return &resourcev1.ResourceSlice{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: resourcev1.ResourceSliceSpec{
			Driver:   driver,
			NodeName: ptr.To(node),
			Pool:     resourcev1.ResourcePool{Name: pool, Generation: generation, ResourceSliceCount: 1},
			Devices:  devices,
		},
	}
}

func TestDevicesByNode(t *testing.T) {
	c := newFakeClient(t,
		slice("n1-gen1", DRADriverName, "node-1", "node-1", 1, device("gpu-0", "amdgpu")),
		slice("n1-gen2", DRADriverName, "node-1", "node-1", 2,
			device("gpu-0", "amdgpu"), device("gpu-1", "amdgpu"), device("gpu-1-p0", "amdgpu-partition")),
		slice("n2", DRADriverName, "node-2", "node-2", 1, device("gpu-0", "amdgpu")),
		slice("nv", "gpu.nvidia.com", "node-3", "node-3", 1, device("gpu-0", "amdgpu")),
	)

	got, err := DevicesByNode(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]int{"node-1": 2, "node-2": 1}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DevicesByNode = %v, want %v (stale gen, partitions and other drivers excluded)", got, want)
	}
}
```

- [ ] **Step 2: Run the tests to confirm they fail**

Run: `go test ./internal/discovery/`
Expected: FAIL, with `undefined: IsAMDGPUNode`.

- [ ] **Step 3: Write `internal/discovery/nodes.go`**

```go
// Package discovery finds AMD GPU nodes and the devices DRA publishes for them.
package discovery

import (
	"context"
	"fmt"
	"regexp"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Labels set by the NodeFeatureRule from AMD's install docs.
const (
	LabelAMDGPU  = "feature.node.kubernetes.io/amd-gpu"
	LabelAMDVGPU = "feature.node.kubernetes.io/amd-vgpu"
)

// nfdAMDPCILabel matches raw NFD PCI labels for AMD (vendor 1002), in either
// the default "<class>_<vendor>" or AMD's "<vendor>_<device>" field layout.
var nfdAMDPCILabel = regexp.MustCompile(`^feature\.node\.kubernetes\.io/pci-([0-9a-f]{4}_)?1002(_[0-9a-f]{4})?\.present$`)

// IsAMDGPUNode reports whether node labels show an AMD GPU.
func IsAMDGPUNode(labels map[string]string) bool {
	if labels[LabelAMDGPU] == "true" || labels[LabelAMDVGPU] == "true" {
		return true
	}

	for k, v := range labels {
		if v == "true" && nfdAMDPCILabel.MatchString(k) {
			return true
		}
	}

	return false
}

// GPUNodes returns every node with an AMD GPU.
func GPUNodes(ctx context.Context, c client.Reader) ([]corev1.Node, error) {
	var nodes corev1.NodeList
	if err := c.List(ctx, &nodes); err != nil {
		return nil, fmt.Errorf("listing nodes: %w", err)
	}

	var gpuNodes []corev1.Node

	for _, n := range nodes.Items {
		if IsAMDGPUNode(n.Labels) {
			gpuNodes = append(gpuNodes, n)
		}
	}

	return gpuNodes, nil
}
```

- [ ] **Step 4: Write `internal/discovery/slices.go`**

```go
package discovery

import (
	"context"
	"fmt"

	resourcev1 "k8s.io/api/resource/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DRADriverName is the AMD DRA driver name and its DeviceClass name.
const DRADriverName = "gpu.amd.com"

// fullGPUType is the "type" attribute value of a whole (unpartitioned) GPU.
const fullGPUType = "amdgpu"

// DevicesByNode counts full AMD GPUs per node from the newest generation of
// each ResourceSlice pool. It returns a NoKindMatch error (check with
// meta.IsNoMatchError) when resource.k8s.io/v1 is not served.
func DevicesByNode(ctx context.Context, c client.Reader) (map[string]int, error) {
	var slices resourcev1.ResourceSliceList
	if err := c.List(ctx, &slices); err != nil {
		return nil, fmt.Errorf("listing ResourceSlices: %w", err)
	}

	latest := map[string]int64{}

	for _, s := range slices.Items {
		if s.Spec.Driver == DRADriverName && s.Spec.Pool.Generation > latest[s.Spec.Pool.Name] {
			latest[s.Spec.Pool.Name] = s.Spec.Pool.Generation
		}
	}

	counts := map[string]int{}

	for _, s := range slices.Items {
		if s.Spec.Driver != DRADriverName || s.Spec.NodeName == nil ||
			s.Spec.Pool.Generation != latest[s.Spec.Pool.Name] {
			continue
		}

		for _, d := range s.Spec.Devices {
			if deviceType(d) == fullGPUType {
				counts[*s.Spec.NodeName]++
			}
		}
	}

	return counts, nil
}

// deviceType returns the device's "type" attribute. Unqualified attribute
// names belong to the driver's domain, so both spellings are accepted.
func deviceType(d resourcev1.Device) string {
	for _, key := range []resourcev1.QualifiedName{"type", DRADriverName + "/type"} {
		if a, ok := d.Attributes[key]; ok && a.StringValue != nil {
			return *a.StringValue
		}
	}

	return ""
}
```

- [ ] **Step 5: Vendor and run the tests**

```bash
make deps-update
go test ./internal/discovery/
```
Expected: `ok`

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat(discovery): find AMD GPU nodes and DRA devices

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 10: `inittools` and the `smoke` suite

> **Status: ✅ DONE** — commit `e4e29cc`. `go test ./...`, `make verify`, and
> tagged smoke-suite compilation passed. The live suite could not run with
> the available kubeconfig because its Minikube certificate files are missing.

This is the first Ginkgo suite. It checks that config loading, platform detection, clients, and discovery all work against a real cluster. It installs nothing.

**Agreed adjustment:** Cluster suites use the `integration` build tag, and
`scripts/test-runner.sh` passes `--tags=integration` to Ginkgo. This keeps
`go test ./...` cluster-independent while preserving startup validation in
`inittools.init()` for integration runs. Compile the smoke suite with
`go test -tags=integration -c`. `make verify` includes integration files in
lint and vet, but does not execute them.

**Files:**
- Create: `internal/inittools/inittools.go`, `tests/smoke/smoke_suite_test.go`, `tests/smoke/smoke_test.go`

- [ ] **Step 1: Write `internal/inittools/inittools.go`**

```go
// Package inittools creates the globals every suite dot-imports: APIClient and
// Config. Config is loaded, completed with platform defaults, and validated
// before any spec runs, so bad settings fail immediately.
package inittools

import (
	"flag"

	"github.com/golang/glog"
	"github.com/johnahull/amd-ci/internal/config"
	"github.com/johnahull/amd-ci/internal/platform"
	"github.com/johnahull/amd-ci/pkg/clients"
	ginkgo "github.com/onsi/ginkgo/v2"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	// APIClient provides access to the cluster.
	APIClient *clients.Settings
	// Config holds validated amd-ci settings.
	Config *config.Config
)

func init() {
	logf.SetLogger(zap.New(zap.WriteTo(ginkgo.GinkgoWriter), zap.UseDevMode(true)))

	var err error

	if Config, err = config.Load(); err != nil {
		glog.Fatalf("loading config: %v", err)
	}

	_ = flag.Lookup("logtostderr").Value.Set("true")
	_ = flag.Lookup("v").Value.Set(Config.VerboseLevel)

	if APIClient, err = clients.New(""); err != nil {
		glog.Fatalf("creating API client (check KUBECONFIG): %v", err)
	}

	p, err := platform.Detect(APIClient.Discovery, Config.Platform)
	if err != nil {
		glog.Fatalf("detecting platform: %v", err)
	}

	Config.ApplyPlatformDefaults(p)

	if err := Config.Validate(); err != nil {
		glog.Fatalf("invalid configuration:\n%v", err)
	}
}
```

- [ ] **Step 2: Write `tests/smoke/smoke_suite_test.go`**

```go
package smoke

import (
	"path/filepath"
	"testing"

	. "github.com/johnahull/amd-ci/internal/inittools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSmoke(t *testing.T) {
	RegisterFailHandler(Fail)

	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = filepath.Join(Config.ReportsDir, "smoke_junit.xml")

	RunSpecs(t, "Smoke", Label("smoke"), reporterConfig)
}
```

- [ ] **Step 3: Write `tests/smoke/smoke_test.go`**

```go
package smoke

import (
	"github.com/johnahull/amd-ci/internal/config"
	"github.com/johnahull/amd-ci/internal/discovery"
	. "github.com/johnahull/amd-ci/internal/inittools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
)

var _ = Describe("Cluster smoke", Label("smoke"), func() {
	It("detects the platform and applies defaults", func() {
		Expect(Config.Platform).To(BeElementOf(config.PlatformOpenShift, config.PlatformKubernetes))
		Expect(Config.Namespace).NotTo(BeEmpty())
		GinkgoWriter.Printf("platform=%s namespace=%s driverMode=%s draSource=%s\n",
			Config.Platform, Config.Namespace, Config.Driver.Mode, Config.DRA.Source)
	})

	It("finds AMD GPU nodes", func(ctx SpecContext) {
		nodes, err := discovery.GPUNodes(ctx, APIClient)
		Expect(err).NotTo(HaveOccurred())

		if len(nodes) == 0 {
			Skip("no AMD GPU nodes: no amd-gpu label and no NFD PCI vendor 1002 label")
		}

		for _, n := range nodes {
			GinkgoWriter.Printf("AMD GPU node: %s\n", n.Name)
		}
	})

	It("reads DRA devices when resource.k8s.io/v1 is served", func(ctx SpecContext) {
		counts, err := discovery.DevicesByNode(ctx, APIClient)
		if meta.IsNoMatchError(err) {
			Skip("resource.k8s.io/v1 is not served by this cluster")
		}

		Expect(err).NotTo(HaveOccurred())
		GinkgoWriter.Printf("AMD DRA devices by node: %v\n", counts)
	})
})
```

- [ ] **Step 4: Vendor, then compile the suite without a cluster**

```bash
go get github.com/onsi/ginkgo/v2@v2.28.1 github.com/onsi/gomega@v1.39.1 github.com/golang/glog@v1.2.5
make deps-update
go vet ./...
go test -tags=integration -c -o /dev/null ./tests/smoke/
```
Expected: `go vet` prints nothing. `go test -c` compiles without errors. The suite isn't run here, because `inittools` exits fatally without a cluster.

- [ ] **Step 5: Run the full unit-test target and lint**

```bash
make unit-test
make lint
```
Expected: `unit-test` shows `ok` for `internal/config`, `internal/platform`, `internal/discovery`, `pkg/amdgpu`, `pkg/amdgpu/v1alpha1`, and `pkg/clients`, and skips `tests/smoke`. `lint` reports `0 issues`. Fix any lint findings before committing.

- [ ] **Step 6 (needs a cluster): Run the smoke suite**

```bash
make install-ginkgo
KUBECONFIG=<path> TEST_FEATURES=smoke TEST_VERBOSE=true make run-tests
```
Expected: 3 specs. "detects the platform" passes and prints the right platform. The GPU node spec passes (or skips on a cluster without AMD GPUs). The DRA spec passes on Kubernetes ≥1.34 / OCP ≥4.21 and skips on older clusters. If no cluster is available, record that this step was not run.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "feat: add inittools and cluster smoke suite

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 11: Reconcile the spec with implementation decisions

> **Status: ✅ DONE** — the six spec corrections below were applied, along
> with documentation of the agreed `integration` build tag.

**Files:**
- Modify: `docs/superpowers/specs/2026-09-25-amd-ci-design.md`

- [ ] **Step 1: Apply these edits to the spec**

1. §2 "Rules", replace the `pkg/amdgpu` bullet with:
   > `pkg/amdgpu/v1alpha1` is a trimmed local copy of AMD's `amd.com/v1alpha1` types (only fields amd-ci uses). The upstream module needs Go 1.26.7 / k8s.io v0.36 and pulls in prometheus-operator. Updates always go through JSON merge patches (`Builder.Apply`), so unmodeled fields are preserved.
2. §3 table, `AMD_DRA_ARGS` row: state the format explicitly as `k=v,k=v`, split on the first `=`.
3. §3 validation rules: custom operator on Kubernetes requires a non-default `AMD_OPERATOR_CHART` **or** `AMD_OPERATOR_IMAGE`.
4. §3 intro: validation runs in `inittools.init()` (before any spec), not `BeforeSuite`.
5. §5 step 2: rename `devicePlugin.enable` to `devicePlugin.enableDevicePlugin` (the real field name).
6. §4 step 1: GPU node detection uses `feature.node.kubernetes.io/amd-gpu` / `amd-vgpu`, or raw NFD PCI labels for vendor `1002`.

- [ ] **Step 2: Commit**

```bash
git add docs/superpowers/specs/2026-09-25-amd-ci-design.md
git commit -m "docs(spec): record foundation implementation decisions

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```
