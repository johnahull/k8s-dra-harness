# amd-ci: Test Framework for AMD GPU Operator and DRA Driver

**Date:** 2026-09-25
**Status:** Draft, awaiting review

## 1. Purpose

A Go + Ginkgo test framework, modeled on
[nvidia-ci](https://github.com/rh-ecosystem-edge/nvidia-ci), that tests the
**AMD GPU Operator** (`ROCm/gpu-operator`) and the **AMD DRA driver**
(`ROCm/k8s-gpu-dra-driver`) against a pre-installed cluster. The cluster can be
**OpenShift or vanilla Kubernetes**.

The framework tests what ships to users: released operators installed the way
a customer would install them. It can also test patched builds of the operator,
the DRA driver, and the kernel driver.

### Why a standalone repo

- AMD's upstream `tests/` suites (gocheck, CI mock images) check AMD's own
  builds. This framework takes a partner/QE view: released artifacts,
  certified platforms, Red Hat-specific concerns (OLM, KMM, OCP versions).
- Keeping it separate from nvidia-ci avoids carrying nvidia-ci's OpenShift-only
  assumptions into the Kubernetes path, and avoids mixing vendors in one repo.
- Regression tests for bugs this framework finds can still be sent upstream in
  AMD's own test framework.

### v1 scope

In scope:
- Deploy: dependencies (NFD, KMM, cert-manager), AMD GPU Operator, `DeviceConfig`
- ROCm workload through the device plugin (`amd.com/gpu`)
- DRA: driver install (through the operator or the standalone Helm chart),
  ResourceSlices, DeviceClass, claim allocation, sharing, and a negative case

Out of scope for v1: GPU partitioning (SPX/CPX/NPS), operator upgrade tests,
metrics exporter / NPD / remediation, KubeVirt / VFIO, dashboard, Prow job
definitions.

## 2. Architecture

### Platform abstraction

A single `Platform` interface isolates every OpenShift/Kubernetes difference.
It is selected once at startup. Suites are written once and call `Platform`
methods. Only `internal/platform/` knows which platform it is running on.

```go
type Platform interface {
    Name() string                                  // "openshift" | "kubernetes"
    InstallNFD(ctx context.Context) error
    InstallDriverDeps(ctx context.Context, mode DriverMode) error
    InstallOperator(ctx context.Context, src OperatorSource) error
    UninstallOperator(ctx context.Context) error
    GrantPrivileged(ctx context.Context, ns, serviceAccount string) error
}
```

| Method | OpenShift | Kubernetes |
|---|---|---|
| `InstallNFD` | OLM Subscription (`openshift-nfd`, `redhat-operators`, `stable`) | NFD subchart of the AMD operator Helm chart |
| `InstallDriverDeps` | KMM via OLM (`openshift-kmm`) when driver mode is `operator` | KMM subchart + cert-manager; no-op when mode is `preinstalled` |
| `InstallOperator` | release: Subscription from catalog; custom: `operator-sdk run bundle` | release: Helm repo chart; custom: local path / `oci://` chart + image override |
| `GrantPrivileged` | SCC binding (privileged) for the service account | PodSecurity `privileged` label on the namespace |

**Detection:** if `AMD_PLATFORM` is set, use it. Otherwise, check whether the
`config.openshift.io` API group is served. On OpenShift → `openshift`,
otherwise → `kubernetes`.

When `AMD_INSTALL_DEPS=false`, `InstallNFD` and `InstallDriverDeps` only
check that the dependencies are present, and fail with a clear message if
they aren't.

### Repo layout

```
amd-ci/
├── Makefile                 # lint, vet, unit-test, run-tests, deps-update, generate
├── Containerfile            # ginkgo + helm + oc/kubectl + operator-sdk
├── .golangci.yml            # same linters as nvidia-ci, incl. revive filename rule
├── scripts/
│   ├── test-runner.sh       # TEST_FEATURES / TEST_LABELS → ginkgo
│   └── must-gather.sh
├── internal/
│   ├── inittools/           # APIClient, Config, Platform globals (dot-imported)
│   ├── config/              # envconfig-based Config + Validate()
│   ├── platform/            # Platform interface, openshift.go, kubernetes.go, detect.go
│   ├── deploy/              # operator install orchestration
│   ├── dra/                 # DRA install (operator | helm) + claim helpers
│   ├── workload/            # ROCm workload pod builders
│   ├── discovery/           # GPU nodes/counts from node labels + ResourceSlices
│   ├── helm/                # Helm SDK wrapper
│   ├── tsparams/            # labels, namespaces and CRDs to dump
│   └── reporter/            # k8sreporter dump on failure
├── pkg/
│   ├── clients/             # kube + dynamic + controller-runtime client
│   ├── amdgpu/              # DeviceConfig builder
│   ├── olm/ nfd/ kmm/
│   └── pod/ namespace/ deployment/ daemonset/ nodes/ resourceclaim/
└── tests/
    ├── amdgpu/              # deploy + workload suite
    └── dra/                 # DRA suite
```

Rules:
- `pkg/clients` registers OpenShift schemes (OLM, config, security) but never
  requires them. Only `internal/platform/openshift.go` uses OpenShift APIs.
- `pkg/amdgpu` imports the typed `DeviceConfig` API from the
  `github.com/ROCm/gpu-operator` Go module (vendored). If that module causes
  dependency conflicts, copy `api/v1alpha1` into the repo instead.
- `pkg/` builders follow nvidia-ci's builder pattern
  (`NewBuilder`/`Pull`/`Create`/`Update`/`Delete`/`Exists`).

## 3. Configuration

All configuration comes from env vars loaded with `envconfig`. Platform-dependent
defaults are applied **after** detection. `Config.Validate()` runs in
`BeforeSuite` and rejects invalid combinations before any install starts.

| Var | Default (OpenShift / Kubernetes) | Purpose |
|---|---|---|
| `KUBECONFIG` | — | Cluster |
| `AMD_PLATFORM` | auto-detect | `openshift` \| `kubernetes` |
| `AMD_CLEANUP` | `true` | `false` keeps installs when chaining suites |
| `AMD_NAMESPACE` | `openshift-amd-gpu` / `kube-amd-gpu` | Operator namespace |
| `AMD_TIMEOUT_SCALE` | `1.0` | Multiplier applied to every default timeout |
| `DUMP_FAILED_TESTS` | `false` | Enable reporter dumps |
| `REPORTS_DUMP_DIR` | `/tmp/reports` | Reporter output (overridden by `ARTIFACT_DIR`) |
| `DRY_RUN` | `false` | Build and log objects without applying them |
| `AMD_OPERATOR_SOURCE` | `release` | `release` \| `custom` |
| `AMD_OPERATOR_VERSION` | latest in channel / latest chart | Pin a release |
| `AMD_OPERATOR_CHANNEL` | `alpha` | OpenShift release channel |
| `AMD_OPERATOR_CATALOG` | `certified-operators` | OpenShift CatalogSource |
| `AMD_OPERATOR_BUNDLE` | — | OpenShift custom: bundle image |
| `AMD_OPERATOR_CHART` | `rocm/gpu-operator-charts` | Kubernetes: repo chart, local path, or `oci://` |
| `AMD_OPERATOR_IMAGE` | — | Kubernetes custom: controller image override |
| `AMD_INSTALL_DEPS` | `true` | `false` = only check that NFD/KMM/cert-manager are present |
| `AMD_DRIVER_MODE` | `operator` / `preinstalled` | Sets `DeviceConfig.spec.driver.enable` |
| `AMD_DRIVER_VERSION` | — | `spec.driver.version` |
| `AMD_DRIVER_IMAGE` | — | `spec.driver.image` |
| `AMD_DRA_SOURCE` | `operator` | `operator` \| `helm` |
| `AMD_DRA_IMAGE` | — | DRA driver image; valid with either source |
| `AMD_DRA_CHART` | `rocm-k8s-gpu-dra-driver/k8s-gpu-dra-driver` | Helm source: repo chart, local path, or `oci://` |
| `AMD_DRA_NAMESPACE` | value of `AMD_NAMESPACE` | Namespace for the Helm-installed DRA driver |
| `AMD_DRA_ARGS` | — | `k=v,k=v` → `cmdLineArguments` (operator) or chart values (helm) |
| `AMD_WORKLOAD_IMAGE` | `docker.io/rocm/dev-ubuntu-22.04:<pinned tag>` | ROCm test container |
| `AMD_WORKLOAD_DURATION` | `60s` | GEMM loop duration |

Validation rules (non-exhaustive):
- `AMD_OPERATOR_SOURCE=custom` requires `AMD_OPERATOR_BUNDLE` on OpenShift, or
  `AMD_OPERATOR_CHART` pointing at a non-default source on Kubernetes.
- `AMD_OPERATOR_BUNDLE` on Kubernetes is an error.
- `AMD_DRA_CHART` set with `AMD_DRA_SOURCE=operator` is an error.

### Patched-component matrix

| Patched component | How to test it |
|---|---|
| DRA driver code only | `AMD_DRA_SOURCE=operator` + `AMD_DRA_IMAGE` |
| DRA code + chart / RBAC / manifests | `AMD_DRA_SOURCE=helm` + `AMD_DRA_CHART=<fork path>` + `AMD_DRA_IMAGE` |
| Operator | `AMD_OPERATOR_SOURCE=custom` + bundle (OpenShift) or chart/image (Kubernetes) |
| Kernel driver | `AMD_DRIVER_MODE=operator` + `AMD_DRIVER_IMAGE`/`AMD_DRIVER_VERSION`, or `preinstalled` with the module already on the host |

## 4. Suite: `tests/amdgpu` (deploy + workload)

One `Ordered` Describe with label `amdgpu`. Each step is its own `It` so it's
clear which step failed.

1. **BeforeAll**
   - `Config.Validate()`
   - `discovery.GPUNodes()` finds nodes with an AMD GPU (PCI vendor `1002`
     NFD label). Skip the suite if there are none.
   - Record pre-existing state: NFD, KMM, cert-manager, operator,
     DeviceConfig, and whether the device plugin is enabled.
2. **installs dependencies** `[deploy]`: `Platform.InstallNFD`, `Platform.InstallDriverDeps`.
3. **installs the AMD GPU operator** `[deploy]`: `Platform.InstallOperator`.
   OLM waits for CSV `Succeeded`, Helm waits for the controller Deployment
   to be Ready.
4. **creates a DeviceConfig** `[deploy]`: driver mode from config, device
   plugin enabled. On OpenShift with the `operator` driver mode, also create
   the AMD `NodeFeatureRule` from AMD's OpenShift docs.
5. **driver is loaded on all GPU nodes** `[deploy]`: DeviceConfig status
   shows ready nodes equal to the number of GPU nodes. Uses the long KMM build
   timeout.
6. **nodes advertise amd.com/gpu** `[deploy]`: allocatable `amd.com/gpu > 0`
   on every GPU node, and the total matches discovery.
7. **runs a ROCm workload** `[workload]`: a pod with 1 `amd.com/gpu` runs
   `rocm-smi` and then a GEMM loop for `AMD_WORKLOAD_DURATION`. The script
   verifies the GEMM result and prints `PASS`. Pass = pod `Succeeded` **and**
   stdout contains `PASS`.
8. **runs a multi-GPU workload** `[workload, multi-gpu]`: skip unless a node
   has ≥2 GPUs. Request 2 GPUs and assert that `rocm-smi` shows 2.
9. **AfterAll** (if `AMD_CLEANUP`): see §7.

**Idempotent setup:** if the operator or DeviceConfig already exists and is
healthy, `BeforeAll` uses it instead of reinstalling. This allows
`TEST_FEATURES=dra` alone, and `amdgpu,dra` chained with `AMD_CLEANUP=false`.

## 5. Suite: `tests/dra`

One `Ordered` Describe with label `dra`. It behaves the same with either DRA source.

1. **BeforeAll**
   - Require `resource.k8s.io/v1` to be served. If only `v1beta*` is
     available (Kubernetes <1.34 / OCP <4.21), skip the suite with a clear
     message.
   - Ensure the operator is present (idempotent, as in §4).
   - `discovery.GPUNodes()`. Skip if there are none.
2. **installs the DRA driver** `[dra, deploy]`
   - `operator`: update the DeviceConfig to `devicePlugin.enable=false` and
     wait for the device-plugin DaemonSet to be removed, then set
     `draDriver.enable=true` plus the image/args from config. The operator
     rejects both enabled at the same time.
   - `helm`: disable the device plugin on the DeviceConfig (if the operator is
     present) and wait for it to be removed, then
     `Platform.GrantPrivileged(ns, sa)`, then Helm install the chart with the
     image and args.
3. **DRA driver pods are Ready on all GPU nodes** `[dra]`
4. **publishes ResourceSlices** `[dra]`: each GPU node has a slice for driver
   `gpu.amd.com`, and its device count matches discovery. These slice counts
   drive every later skip decision.
5. **DeviceClass gpu.amd.com exists** `[dra]`
6. **no amd.com/gpu extended resource remains** `[dra]`: checks that the
   migration away from the device plugin happened.
7. **allocates a single GPU via ResourceClaimTemplate** `[dra, dra-single]`:
   `rocm-smi` sees exactly 1 GPU and the claim is allocated. After the pod is
   deleted, the claim is released.
8. **allocates 2 GPUs on the same PCIe root** `[dra, dra-multi]`: skip unless
   slices show ≥2 devices on one root. Uses CEL selectors + `matchAttribute`,
   following the driver's `example-two-gpus-same-pcieroot.yaml`.
9. **shares one claim across two pods** `[dra, dra-share]`: one
   `ResourceClaim` is referenced by 2 pods. Both reach Running and report the
   same GPU ID.
10. **unschedulable claim stays Pending** `[dra, negative]`: request
    (available GPU count + 1). The pod stays Pending with a scheduler DRA
    event, and nothing is partially allocated.
11. **AfterAll** (if `AMD_CLEANUP`): delete the test namespace, uninstall DRA
    (`helm uninstall`, or `draDriver.enable=false`), and re-enable the device
    plugin if it was enabled before.

Device selectors in v1 rely only on `device.attributes["gpu.amd.com"].type ==
"amdgpu"`. Detailed attribute checks are deferred because the driver's
device/counter publishing is still changing (KEP-4815 work).

## 6. Hardware assumptions

The target is **mixed/unknown lab hardware**. `internal/discovery` gets GPU
counts and topology from node labels and ResourceSlices. Tests that need more
hardware than exists (multi-GPU, same-PCIe-root) **skip rather than fail**,
and the skip message says what was missing.

## 7. Error handling and cleanup

- **Waits report the last observed state.** `Wait*` helpers return an error
  describing that state, e.g. `DeviceConfig ready 2/3 nodes; node-x: KMM
  build pod ImagePullBackOff`, not a bare `false`.
- **Default timeouts:** operator install 10m, KMM driver build 30m, DRA pods
  5m, workload 10m. All are multiplied by `AMD_TIMEOUT_SCALE`.
- **Cleanup** runs every step even if an earlier one fails, collects all the
  errors, and reports them together. It **never removes anything that existed
  before the suite started**. Order: workload namespaces → DRA → DeviceConfig
  (wait for driver unload) → operator → dependencies this run installed.

## 8. Diagnostics

- **Reporter** (k8sreporter, from nvidia-ci) runs on failure when
  `DUMP_FAILED_TESTS=true`.
  - Namespaces: the operator namespace, the DRA namespace, and the test namespaces.
  - CRDs: `DeviceConfig`, KMM `Module`, `NodeFeatureRule`, `ResourceSlice`,
    `ResourceClaim`, `DeviceClass`. On OpenShift, also `Subscription`, `CSV`,
    `InstallPlan`.
- **`scripts/must-gather.sh`** runs on suite failure and collects:
  - pod logs, including previous containers
  - node `describe`
  - `dmesg | grep amdgpu` from a debug pod
  - `helm get values` for Helm-installed releases

## 9. Testing the framework

- **Unit tests** (`make unit-test`, no cluster needed):
  - config validation and platform defaults
  - `DeviceConfig` builder output
  - Helm values rendering for operator/DRA installs
  - discovery parsing of ResourceSlice fixtures
  - `Platform` implementations against a fake controller-runtime client, with
    OLM/Helm calls mocked via `mockgen`
- **`DRY_RUN=true`**: every suite builds and logs its objects without applying
  them.
- **Repo CI**: GitHub Actions runs `make lint vet unit-test`. GPU cluster
  runs are triggered separately (Prow or lab) and are outside v1 scope.

## 10. Running

```bash
# Kubernetes, released operator, deploy + DRA, keep installs for inspection
TEST_FEATURES="amdgpu dra" AMD_CLEANUP=false make run-tests

# OpenShift, patched DRA driver from a local fork chart
TEST_FEATURES=dra AMD_DRA_SOURCE=helm \
  AMD_DRA_CHART=~/redhat/amd/k8s-gpu-dra-driver/helm-charts-k8s \
  AMD_DRA_IMAGE=quay.io/<user>/k8s-gpu-dra-driver:kep4815 \
  make run-tests

# Only single-GPU DRA tests
TEST_FEATURES=dra TEST_LABELS='dra-single' make run-tests
```
