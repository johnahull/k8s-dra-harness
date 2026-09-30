# k8s-dra-harness user guide

This guide explains how to run the harness against Kubernetes or OpenShift,
choose a driver, use shared-cluster safety modes, run KubeVirt checks, and
configure the reusable DRA test plans.

The harness is a test tool, not a DRA driver or an operator. It can build and
install selected driver checkouts, verify an existing installation, create
short-lived test claims and workloads, and clean up resources owned by its run.

## Before you start

### Required tools

Install or make available on `PATH`:

- Go, for unit tests and source builds.
- `kubectl` and a working `KUBECONFIG`.
- Helm for chart installation.
- Docker for the built-in source-build adapters (the SR-IOV adapter also
  honors its checkout's `CONTAINER_TOOL` setting), plus the selected
  checkout's own build dependencies.
- Ginkgo v2 for the integration suites. Run `make install-ginkgo` if it is
  not already installed.
- `oc` for OpenShift runs. AMD Operator bundle installs additionally require
  `operator-sdk` and OLM.
- `virtctl` when KubeVirt guest verification is configured.

The target cluster must serve the GA DRA API, `resource.k8s.io/v1`. It must
also be able to pull the selected workload and driver images. Source builds
need a registry where the harness can push the generated image.

Before a run, verify the context explicitly:

```sh
export KUBECONFIG=/absolute/path/to/kubeconfig
kubectl config current-context
kubectl cluster-info
kubectl api-resources --api-group=resource.k8s.io
```

The harness uses the current `KUBECONFIG`; there is no separate kubeconfig
field in the YAML file. Confirm the context before using any mode that creates
resources.

### Hardware and runtime prerequisites

| Adapter | GPU hardware required? | Additional requirements |
| --- | --- | --- |
| `example` | No | A checkout of the Kubernetes DRA example driver and a pushable registry. |
| `cpu` | No | CPU DRA driver, Node Resource Interface (NRI)/Container Device Interface (CDI) support, and compatible CPUManager configuration. KubeVirt CPU runs additionally need `CPUsWithDRA`, manual all-resource claim support, and Kubernetes consumable-capacity support (`DRAConsumableCapacity`). |
| `amd` | Yes | AMD GPU nodes with a working `amdgpu` kernel driver and the AMD DRA driver. |
| `nvidia` | Yes | NVIDIA GPU Operator prerequisites, NVIDIA driver/toolkit, and the standalone NVIDIA DRA driver. |
| `sriov` | No GPU, but SR-IOV NICs are required | SR-IOV-capable nodes, the driver's CDI/runtime prerequisites, and a matching `SriovResourcePolicy`. Multus is needed for `MULTUS` mode; `STANDALONE` mode additionally needs NRI and a NetworkAttachmentDefinition configuration. KubeVirt runs additionally need `NetworkDevicesWithDRA` and a pre-created NetworkAttachmentDefinition (NAD). |

The `example` adapter is the recommended first validation because it advertises
mock devices and does not require a GPU. NVIDIA support is experimental and
should be validated against pinned driver and operator revisions.

## The normal test flow

Run commands from the repository root. The test runner selects Ginkgo suites
with `TEST_FEATURES` and reads the YAML path from `DRA_HARNESS_CONFIG`:

```sh
make unit-test
make install-ginkgo

KUBECONFIG=/absolute/path/to/kubeconfig \
DRA_HARNESS_CONFIG=examples/example.yaml \
TEST_FEATURES=dra \
make run-tests
```

Useful local checks are:

```sh
make unit-test
make verify
go test -tags=integration ./tests/dra -run '^$'
```

The last command compiles the DRA suite without connecting to a cluster. A
live suite run requires `DRA_HARNESS_CONFIG`, `KUBECONFIG`, and the selected
cluster prerequisites.

`TEST_FEATURES` accepts one or more comma- or space-separated suite directories
under `tests`, or `all`. For example:

```sh
TEST_FEATURES="smoke dra" DRA_HARNESS_CONFIG=examples/example.yaml make run-tests
```

Set `TEST_VERBOSE=true` or `TEST_TRACE=true` for more Ginkgo output. Set
`ARTIFACT_DIR=/path/to/artifacts` to write the smoke JUnit report there.

## Run configuration

Configuration is strict YAML: unknown fields are rejected. Paths beginning
with `.` are resolved relative to the YAML file, not the shell's current
directory.

The basic installation shape is:

```yaml
namespace: dra-harness
registry: quay.io/my-account
cleanup: true
drivers:
  - name: example
    sourcePath: /path/to/dra-example-driver
```

Each driver selects exactly one artifact source:

- `sourcePath` builds and pushes the checkout image. `registry` is required.
- `image` uses an already-published, explicitly tagged image. `chart` is also
  required when using an image.
- `namespace` optionally selects the driver namespace. Otherwise the harness
  derives a run-owned namespace.
- `upstreamTests` is an argv list, not a shell command. It runs from the
  driver checkout after deployment and receives `DRA_HARNESS_IMAGE` and
  `DRA_HARNESS_NAMESPACE`.
- `podSelector` identifies existing driver pods for the test-plan `restart`
  scenario. It is required for existing-driver restart tests.
- `drivers[].claimConfig` adds opaque driver parameters to that driver's regular
  pod workload ResourceClaims. It is useful when a driver needs per-claim
  configuration.

The `sriov` adapter accepts an optional `sriovPolicy` block. The harness creates
the namespaced `SriovResourcePolicy` after installing the driver chart and
deletes it during cleanup:

```yaml
drivers:
  - name: sriov
    sourcePath: /path/to/dra-driver-sriov
    sriovPolicy:
      name: all-devices
      spec:
        configs:
          - {}
```

If `sriovPolicy.namespace` is omitted, the policy is created in the driver's
run-owned namespace. If it is set while `driver.namespace` is omitted, it also
selects that namespace for the driver; otherwise the two namespaces must
match. The example policy above is intentionally broad; use the SR-IOV
driver's policy selectors to restrict which VFs are advertised on a real
cluster. Without a matching policy, the driver publishes no devices.
For the generic pod allocation check, the harness defaults the chart's
`kubeletPlugin.configurationMode` to `MULTUS`, avoiding a fabricated network
attachment. To exercise `STANDALONE`, provide the driver's opaque claim
configuration and a pre-existing NetworkAttachmentDefinition; the harness does
not create the NAD:

```yaml
drivers:
  - name: sriov
    image: quay.io/example/dra-driver-sriov:dev
    chart: /path/to/dra-driver-sriov/deployments/helm/dra-driver-sriov
    values:
      kubeletPlugin:
        configurationMode: STANDALONE
    claimConfig:
      requests: [device]
      driver: sriovnetwork.k8snetworkplumbingwg.io
      parameters:
        apiVersion: sriovnetwork.k8snetworkplumbingwg.io/v1alpha1
        kind: VfConfig
        ifName: net1
        netAttachDefName: vf-test1
        netAttachDefNamespace: sriov-network-config
```

For `STANDALONE`, create `vf-test1` as a NetworkAttachmentDefinition in
`sriov-network-config` before the run. The harness creates a run-specific
workload namespace, so omitting `netAttachDefNamespace` would make the driver
look for the NAD in that temporary namespace.

The `MULTUS` path is the default because it can exercise generic DRA
allocation without requiring that network attachment configuration.

Complete example: [examples/sriov.yaml](../examples/sriov.yaml).
Its source build honors `CONTAINER_TOOL=podman` as well as the upstream Docker
default.

The harness refuses to install over an existing DeviceClass or Helm release
with the same run name. For an installation run, `cleanup` defaults to true.
Set `cleanup: false` when you need to inspect the installation after the suite.
If Helm cannot determine a release's state during cleanup, the harness reports
the error and retains the owned namespace for inspection. Ownership is not
persisted for a later cleanup-only retry; resolve the Helm/API problem and
inspect the retained release and namespace manually.

## Shared-cluster modes

Use one of these modes deliberately:

### Install and test

This is the default. The harness builds or selects the driver image, installs
the optional operator and driver chart, checks readiness, runs configured
upstream tests, runs the optional DRA test plan, and runs live workload checks.

Example: [examples/example.yaml](../examples/example.yaml).

### Existing-driver validation

Set `existing: true` and provide the namespace of every pre-existing driver:

```yaml
existing: true
drivers:
  - name: amd
    namespace: openshift-amd-gpu
```

Without a test plan, this mode is read-only. It checks the DRA API,
DeviceClass, and nonempty ResourceSlices, and skips installation, upstream
tests, workloads, and cleanup.

Example: [examples/existing-amd.yaml](../examples/existing-amd.yaml).

An existing-driver run can opt into the DRA test plan, but mutations require:

```yaml
testPlan:
  lifecycle:
    allowWorkloads: true
```

The harness then uses a run-specific workload namespace. It does not modify
the pre-existing driver namespace. `allowRestart: true` is a separate explicit
permission to delete and wait for replacement driver pods.

### Preflight

Set `preflight: true` with the normal driver artifact configuration:

```yaml
preflight: true
drivers:
  - name: cpu
    image: quay.io/example/dra-driver-cpu:dev
    chart: oci://quay.io/example/dra-driver-cpu-chart
```

Preflight checks cluster/API availability, DeviceClass collisions, Helm
release-name availability, and platform prerequisites. It does not build,
install, create workloads, or run a test plan.

Example: [examples/cpu-preflight.yaml](../examples/cpu-preflight.yaml).

`existing` and `preflight` are mutually exclusive.

## Operators and NVIDIA

An `amdOperator` block installs the supplied AMD GPU Operator chart on
Kubernetes or a bundle on OpenShift. Bundle runs require `package` and a raw
`deviceConfig` specification so cleanup can identify the run's DeviceConfig.
See [examples/amd-operator.yaml](../examples/amd-operator.yaml).

An `nvidiaOperator` block installs the NVIDIA GPU Operator as a prerequisite
for the standalone NVIDIA DRA driver. When the `nvidia` adapter is selected,
the harness disables the operator's legacy device plugin to avoid two
allocators competing for the same GPUs. The operator's `GPUCluster` DRA mode
is intentionally rejected by this harness.

Examples:

- [combined NVIDIA operator and DRA driver](../examples/nvidia.yaml)
- [operator-only NVIDIA run](../examples/nvidia-operator.yaml)

## KubeVirt and OpenShift Virtualization

KubeVirt testing is a separate workload mode. The cluster must already have
KubeVirt or OpenShift Virtualization installed, Kubernetes DRA enabled, and
the feature gate matching the attachment. GPU and HostDevice use
`GPUsWithDRA` and `HostDevicesWithDRA`; SR-IOV uses `NetworkDevicesWithDRA`.
CPU requires `CPUsWithDRA` and KubeVirt's manual all-resource claim support.
The harness does not install KubeVirt or change its feature gates. The
`example` adapter is intentionally pod-only.

Use `workload: kubevirt` and a `kubevirt` block:

```yaml
workload: kubevirt
kubevirt:
  namespace: dra-kubevirt
  image: quay.io/containerdisks/fedora:latest
  attachment: gpu
  cloudInitSecret: dra-kubevirt-cloudinit
  guest:
    username: fedora
    privateKeySecret: dra-kubevirt-ssh
    command: "nvidia-smi -L && echo PASS"
    expectedOutput: PASS
```

The harness creates a direct VMI and DRA ResourceClaim in the selected
namespace. Guest verification uses `virtctl ssh`; the private key is copied
to a protected temporary file and is not added to the workload object.

Use [examples/kubevirt-nvidia.yaml](../examples/kubevirt-nvidia.yaml) as a
starting point. CPU and SR-IOV use the same direct-VMI backend with different
attachment fields:

```yaml
# CPU: grouped CPU DRA capacity
kubevirt:
  attachment: cpu
  claimConfig:
    capacity:
      dra.cpu/cpu: "1"

# SR-IOV: DRA-backed KubeVirt network
kubevirt:
  attachment: network
  claimConfig:
    requests: [device]
    driver: sriovnetwork.k8snetworkplumbingwg.io
    parameters:
      apiVersion: sriovnetwork.k8snetworkplumbingwg.io/v1alpha1
      kind: VfConfig
      driver: vfio-pci
      netAttachDefName: sriov-net
      netAttachDefNamespace: dra-kubevirt
```

The CPU example requires a KubeVirt build containing `CPUsWithDRA` and manual
all-resource claim support, plus Kubernetes consumable-capacity support
(`DRAConsumableCapacity`). The SR-IOV claim's `VfConfig` must match a
pre-created NetworkAttachmentDefinition and the installed SR-IOV policy. The
harness creates the DRA claim and VMI, but does not create the NAD or change
KubeVirt feature gates. KubeVirt test plans are intentionally not combined
with the regular DRA `testPlan`; run the KubeVirt workload and the
namespace-scoped DRA scenarios as separate configurations.

## DRA test plans

`testPlan` adds reusable claim-level checks after driver setup. It is useful for
validating an existing driver without adopting that driver's repository-specific
test suite.

`profile` is a human-readable identifier for the plan; the explicit
`scenarios` list controls what runs. Set `workloadImage` when the default test
image is not available in the target registry.

For regular pod claims, `selector` supplies the default CEL device selector and
`claimConfig` supplies an optional opaque DRA device configuration. The latter
is useful for drivers such as AMD's `VfioDeviceConfig` backend-policy tests:

```yaml
testPlan:
  selector: 'device.attributes["gpu.amd.com"].type == "vfio"'
  claimConfig:
    requests: [device]
    driver: gpu.amd.com
    parameters:
      apiVersion: gpu.resource.amd.com/v1alpha1
      kind: VfioDeviceConfig
      iommu:
        backendPolicy: PreferIommuFD
```

The minimum structure is:

```yaml
testPlan:
  profile: amd-pr-91-topology
  scenarios:
    - resource-slices
    - counters
    - sibling-exclusion
    - release
    - topology
  lifecycle:
    allowWorkloads: true
```

Scenario behavior:

| Scenario | What it checks | Mutation |
| --- | --- | --- |
| `resource-slices` | ResourceSlices are published. | Read-only. |
| `counters` | Shared counters are published and can be rendered by `dra-counters.py`. | Read-only. |
| `sibling-exclusion` | An AMD compute allocation prevents allocation of its matching VF sibling. | Creates claims and pods; AMD-only. |
| `capacity` | AMD allocation eventually produces an expected-pending claim. | Creates up to 32 claims and pods; AMD-only. |
| `release` | Deleting a consumer releases its allocation. | Creates and deletes a claim and pod. |
| `topology` | One claim with multiple requests can co-place devices using `matchAttribute`. | Creates a claim or claim template and pod. |
| `restart` | Selected driver pods can restart and resume ResourceSlice publication. | Deletes matching driver pods; requires `allowWorkloads` and `allowRestart`. |

`resource-slices` and `counters` are the only scenarios permitted without
`lifecycle.allowWorkloads: true`. This protection applies even to an
installation run. The harness cleans up its test pods, claims, and templates
after each scenario and never deletes cluster-scoped DeviceClasses or
ResourceSlices.

### Topology cases

Each topology case must have at least two requests. Device classes can come
from different drivers:

```yaml
testPlan:
  profile: amd-pr-91-topology
  scenarios: [topology]
  lifecycle:
    allowWorkloads: true
  topology:
    - name: gpu-cpu-numa
      matchAttribute: resource.kubernetes.io/numaNode
      expected: success
      requests:
        - name: gpu
          deviceClass: gpu.amd.com
        - name: cpu
          deviceClass: dra.cpu
```

Set `expected: pending` for a deliberately unsatisfiable case. Set
`useTemplate: true` to exercise a `ResourceClaimTemplate` referenced by the
consumer pod instead of a directly-created ResourceClaim. A request's
`selector` is a CEL device selector and `count` defaults to one.

### Verification scripts and evidence

The optional `verification` block integrates the read-only tools from the
topology-aware co-placement checkout:

```yaml
verification:
  scriptsDir: /home/user/devel/dra-topology-aware-co-placement/testing/scripts
  expectedRepoCommit: 0123456789abcdef0123456789abcdef01234567
  evidenceDir: /tmp/dra-harness-evidence
```

When `scriptsDir` is set:

- `evidenceDir` is required.
- The checkout must be a Git working tree.
- `expectedRepoCommit`, when set, must match `HEAD`, and the checkout must be
  clean.
- With the default command set, the harness invokes `dra-verify.sh` and
  `show-dra-topology.sh` for applicable scenarios, and invokes
  `dra-counters.py` for the `counters` scenario. Custom commands replace the
  default verifier list. Commands run without a shell; their stdout/stderr is
  saved as evidence.
- ResourceSlice and all-namespace ResourceClaim JSON snapshots are saved for
  the relevant scenario.

Custom `verification.commands` are argv arrays, not shell strings. They are
resolved inside `scriptsDir`; commands outside that checkout are rejected.
For example, obtain a checkout pin with `git -C /path/to/checkout rev-parse
HEAD`. The broad-cleanup demo script is not called by the harness.

## OpenShift notes

OpenShift runs need `oc` available locally. When the harness creates a
namespace, it applies the privileged pod-security label. It grants the
privileged SCC only to created non-workload namespaces, such as driver and
operator namespaces; it does not grant that SCC to the workload namespace.
Existing-driver test-plan runs perform platform detection before creating their
temporary namespace.

Test-plan cleanup never deletes cluster-scoped DeviceClasses or ResourceSlices.
Bundle cleanup passes `--delete-crds=false`, retaining shared CRDs. Helm cleanup
uninstalls only run-owned releases, so cluster-scoped resources owned by those
charts follow Helm's normal uninstall behavior; do not assume chart-owned
cluster-scoped objects are retained.

## Troubleshooting

### The suite says the DRA API is missing

Check the context and API discovery:

```sh
kubectl config current-context
kubectl api-resources --api-group=resource.k8s.io
```

The cluster must expose `resource.k8s.io/v1`; a cluster with only an older
alpha DRA API is not sufficient for this harness.

### No ResourceSlices or DeviceClass

Check the driver namespace and driver pods:

```sh
kubectl get deviceclasses
kubectl get resourceslices
kubectl get pods -A | grep -E 'dra|gpu|nvidia'
```

For an existing-driver run, the configured adapter name and namespace must
match the installed driver. For an install run, inspect the Helm release and
driver pod events before retrying.

### A workload remains Pending

Inspect the pod, claim, and scheduler events:

```sh
kubectl get pod -n <workload-namespace> -o wide
kubectl get resourceclaim -n <workload-namespace> -o yaml
kubectl describe pod -n <workload-namespace> <pod-name>
```

Common causes are missing DeviceClasses, an unavailable device, a selector that
matches no device, missing NRI/CDI support, or an image that cannot be pulled.
For an expected-pending test, the harness intentionally waits for the claim to
remain unallocated before considering the scenario successful.

### Cleanup failed

Do not delete broad namespaces or all claims on a shared cluster. First inspect
the run-specific namespace and Helm state. Installation runs retain owned
namespaces when Helm state cannot be confirmed, but the harness does not
persist ownership or provide a cleanup-only retry command after the process
exits. Resolve the underlying Helm or API problem, then inspect the recorded
release and namespace and clean up only those run-owned resources using the
normal Helm/Kubernetes tools.

### Test-plan verifier failed

Check the evidence directory, recorded verifier commit, and script stderr:

```sh
ls -la /tmp/dra-harness-evidence
cat /tmp/dra-harness-evidence/verification-repo-commit.txt
```

The verifier scripts use the same `KUBECONFIG` as the harness. Confirm that
the invoking identity can list ResourceSlices and the ResourceClaims needed by
the selected checks.

## Related documentation

- [Project README](../README.md) — short overview and configuration examples.
- [Adapter authoring](adapter-authoring.md) — add or extend a built-in driver adapter.
- [Development TODOs](../TODO.md) — known hardware validation and follow-up work.
