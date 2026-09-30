# k8s-dra-harness

`k8s-dra-harness` is an end-to-end test harness for Kubernetes Dynamic
Resource Allocation (DRA) drivers. It runs against an existing Kubernetes or
OpenShift cluster and can:

- build and push a driver checkout, or use a published image and Helm chart;
- install and validate AMD, NVIDIA, CPU, SR-IOV, or example DRA drivers;
- install selected AMD or NVIDIA GPU Operator prerequisites;
- validate a driver that is already installed on a shared cluster;
- run DRA claims, workloads, topology, capacity, release, and restart checks;
- create direct KubeVirt VMIs that consume DRA claims; and
- collect ResourceClaim, ResourceSlice, verifier, and command evidence.

The harness is a test tool. It is not a DRA driver, GPU operator, KubeVirt
installer, cluster provisioner, or general-purpose cleanup tool. KubeVirt and
OpenShift Virtualization must already be installed when those workloads are
used.

## How a run works

An installation run follows this lifecycle:

1. Load and validate a strict YAML configuration.
2. Check the cluster API, platform, artifact collisions, and prerequisites.
3. Build and push source checkouts, when `sourcePath` is used.
4. Install selected operators and driver Helm charts.
5. Wait for DeviceClasses, ResourceSlices, and driver pods to become ready.
6. Run optional upstream commands from the driver checkout.
7. Run the configured DRA test plan and live workload checks.
8. Remove resources owned by the run when cleanup is enabled.

The default installation path uses a unique run ID for derived namespaces,
Helm releases, claims, and workloads. Helm-owned cluster-scoped resources
follow Helm's normal uninstall behavior, so shared charts and operators should
be reviewed before installation on a shared cluster.

## Supported adapters and workloads

Adapters are compiled into the harness; they are not dynamically loaded
plugins.

| Adapter | Purpose | Hardware | Workloads | Status |
| --- | --- | --- | --- | --- |
| `example` | Kubernetes DRA mock-device example driver | No GPU required | Pod | Recommended first run |
| `cpu` | CPU DRA driver | CPUManager plus NRI/CDI support | Pod or KubeVirt (feature-gated) | Hardware/runtime dependent |
| `amd` | AMD GPU DRA driver | AMD GPU and `amdgpu` | Pod or KubeVirt | Hardware dependent |
| `nvidia` | NVIDIA GPU DRA driver | NVIDIA GPU, driver, and toolkit | Pod or KubeVirt | Experimental |
| `sriov` | SR-IOV VF DRA driver | SR-IOV-capable NIC, CDI, and driver runtime prerequisites | Pod or KubeVirt | Hardware/network dependent |

The AMD and NVIDIA GPU Operator configurations can also be used without a DRA
driver for operator/device-plugin validation. Combined multi-driver workloads
run only when the harness has a registered joint-workload policy for that pair.

## Choose a run mode

| Mode | What it does | Cluster mutations |
| --- | --- | --- |
| Install (default) | Builds or selects artifacts, installs them, and runs checks | Yes; limited to resources owned by the run |
| `existing: true` | Validates an installed driver and its DRA resources | Read-only by default |
| `preflight: true` | Checks API access, collisions, release names, and prerequisites | No build, install, workload, or test-plan mutations |

Existing-driver test plans are an explicit exception to the read-only default:
`testPlan.lifecycle.allowWorkloads: true` permits temporary claims and pods,
and `allowRestart: true` additionally permits matching driver pods to be
restarted. Existing KubeVirt workloads require the separate
`kubevirt.allowExisting: true` opt-in.

## Before the first run

Install or make available on `PATH`:

- Go, `kubectl`, Helm, and a working `KUBECONFIG`;
- Docker and a registry login for source builds;
- Ginkgo v2 (`make install-ginkgo` installs the pinned version);
- `oc` for OpenShift runs;
- `operator-sdk` and OLM for AMD Operator bundle installs; and
- `virtctl` when KubeVirt guest verification is configured.

Driver and DRA workload runs require the GA DRA API, `resource.k8s.io/v1`. All
runs must be able to pull their workload and driver images. Operator-only runs
use the selected device-plugin path and may not need the DRA API. Additional
requirements depend on the adapter:

- `example`: a checkout of the Kubernetes DRA example driver and a pushable
  registry;
- `cpu`: a CPU DRA driver with compatible CPUManager, Node Resource Interface
  (NRI), and Container Device Interface (CDI) support. KubeVirt CPU runs also
  require a KubeVirt build with `CPUsWithDRA` and manual all-resource claim
  support, plus Kubernetes consumable-capacity support (`DRAConsumableCapacity`);
- `amd`: AMD GPU nodes with a working `amdgpu` kernel driver and AMD DRA
  driver; and
- `nvidia`: NVIDIA GPU nodes with a working driver and toolkit, plus the
  standalone NVIDIA DRA driver. The NVIDIA GPU Operator is optional unless it
  is selected as the driver prerequisite; and
- `sriov`: SR-IOV-capable nodes, the SR-IOV DRA driver's CDI/runtime
  prerequisites, and a matching `SriovResourcePolicy`. Multus is needed for
  the driver's `MULTUS` mode; `STANDALONE` mode additionally needs NRI and a
  NetworkAttachmentDefinition configuration. KubeVirt SR-IOV runs require
  the `NetworkDevicesWithDRA` feature gate and a pre-created
  NetworkAttachmentDefinition (NAD) referenced by the claim's `VfConfig`.

Confirm the kubeconfig context before using a mode that creates resources:

```sh
export KUBECONFIG=/absolute/path/to/kubeconfig
kubectl config current-context
kubectl cluster-info
kubectl api-resources --api-group=resource.k8s.io
```

## Quick start: hardware-free validation

The `example` adapter advertises mock devices and is the best first run when a
GPU cluster is not available.

1. Edit [examples/example.yaml](examples/example.yaml), replacing
   `sourcePath` with a checkout of the Kubernetes DRA example driver and
   `registry` with a registry where Docker can push and the cluster can pull.
2. From the repository root, run:

   ```sh
   make unit-test
   make install-ginkgo

   KUBECONFIG=/absolute/path/to/kubeconfig \
   DRA_HARNESS_CONFIG=examples/example.yaml \
   TEST_FEATURES=dra \
   make run-tests
   ```

The run builds the driver, installs its chart, checks DRA resources, allocates
a mock device through a pod workload, and cleans up by default. Set
`cleanup: false` when you need to inspect the installation after the run.

## Common workflows

### Validate an already-installed driver

Use `existing: true` when the driver was installed by another system or when
the cluster is shared. The basic mode does not create namespaces, workloads,
claims, or driver resources:

```yaml
existing: true
drivers:
  - name: amd
    namespace: openshift-amd-gpu
```

Run it with the same `TEST_FEATURES=dra make run-tests` command, pointing
`DRA_HARNESS_CONFIG` at the file. The adapter name and namespace must match the
installed driver, and the cluster must expose its DeviceClass and nonempty
ResourceSlices.

### Check an installation before changing the cluster

Use preflight mode with the image and chart you intend to deploy:

```yaml
preflight: true
drivers:
  - name: cpu
    image: quay.io/example/dra-driver-cpu:dev
    chart: oci://quay.io/example/dra-driver-cpu-chart
```

Preflight performs discovery and collision checks, then exits without building,
installing, creating workloads, or running a test plan.

### Exercise a GPU Operator

Use an operator-only configuration when you want to validate the operator's
device-plugin path without selecting a standalone DRA driver:

- [AMD Operator example](examples/amd-operator.yaml)
- [NVIDIA Operator example](examples/nvidia-operator.yaml)

Use [examples/nvidia.yaml](examples/nvidia.yaml) when the NVIDIA GPU Operator
is a prerequisite for the standalone NVIDIA DRA driver.

For SR-IOV, start with [examples/sriov.yaml](examples/sriov.yaml). The example
creates a broad policy (`configs: [{}]`) so the driver can advertise matching
devices; narrow that policy for a real cluster. The harness defaults the chart
to `MULTUS` mode for its generic allocation smoke workload because that
workload does not create a NetworkAttachmentDefinition. Override
`drivers[].values.kubeletPlugin.configurationMode` when testing a complete
SR-IOV network setup and provide `drivers[].claimConfig` for the driver's
opaque per-claim parameters. The SR-IOV source build honors `CONTAINER_TOOL`
(for example, set it to `podman`); otherwise the upstream Makefile defaults to
Docker.

### Run KubeVirt or OpenShift Virtualization checks

The cluster must already have KubeVirt or OpenShift Virtualization, Kubernetes
DRA, and the feature gate for the selected attachment. GPU and HostDevice use
`GPUsWithDRA` and `HostDevicesWithDRA`; SR-IOV uses `NetworkDevicesWithDRA`.
CPU uses `CPUsWithDRA` plus KubeVirt's manual all-resource claim support. The
harness creates a direct VMI and ResourceClaim; it does not install KubeVirt or
change feature gates. The `example` adapter remains pod-only.

Start with [examples/kubevirt-nvidia.yaml](examples/kubevirt-nvidia.yaml).
KubeVirt guest verification additionally needs a cloud-init Secret containing
the SSH public key and a private-key Secret. The configured guest command runs
through `virtctl ssh` after the VMI starts.

For a driver already installed on the cluster, use
[examples/existing-amd-kubevirt-gim-vf.yaml](examples/existing-amd-kubevirt-gim-vf.yaml)
or its held-VMI variant
[examples/existing-amd-kubevirt-gim-vf-k03.yaml](examples/existing-amd-kubevirt-gim-vf-k03.yaml).
Set `kubevirt.allowExisting: true`; this permits the harness to create and
remove only its temporary workload resources. It does not unbind PCI devices,
rebind PFs, or change the installed driver.

`selector` and `claimConfig` can express driver-specific allocation, such as a
pre-bound AMD GIM VFIO VF:

```yaml
existing: true
workload: kubevirt
kubevirt:
  allowExisting: true
  namespace: dra-kubevirt-gim-vf
  image: quay.io/containerdisks/fedora:42
  attachment: gpu
  selector: 'device.attributes["gpu.amd.com"].type == "vfio"'
  claimConfig:
    driver: gpu.amd.com
    parameters:
      apiVersion: gpu.resource.amd.com/v1alpha1
      kind: VfioDeviceConfig
      iommu:
        backendPolicy: LegacyOnly
drivers:
  - name: amd
    namespace: dra-pr122-vfio-lifecycle
```

KubeVirt workloads support AMD and NVIDIA GPU/HostDevice attachments, CPU DRA
claims, and SR-IOV DRA network attachments. CPU support depends on a KubeVirt
build that includes `CPUsWithDRA` and manual all-resource claim support; a
stock KubeVirt build without that feature is rejected during preflight. The
`example` adapter is intentionally pod-only. KubeVirt workloads cannot be
combined with the regular DRA `testPlan` in the same configuration.

For CPU, select `attachment: cpu`; the harness requests grouped CPU capacity
(`dra.cpu/cpu`) and adds KubeVirt's manual-claim annotation. For SR-IOV,
select `attachment: network`; the harness adds a DRA-backed KubeVirt network
with an SR-IOV interface. See the adapter-specific configuration in the
[user guide](docs/user-guide.md#kubevirt-and-openshift-virtualization).

### Run reusable DRA test plans

`testPlan` adds namespace-scoped claim and allocation scenarios after driver
setup. Available scenarios are:

- `resource-slices` and `counters`: read-only publication checks;
- `sibling-exclusion` and `capacity`: AMD-specific allocation checks;
- `release`: verifies that deleting a consumer releases its allocation;
- `topology`: tests multi-request placement and optional claim templates; and
- `restart`: restarts selected driver pods and checks ResourceSlice recovery.

Mutating scenarios require `lifecycle.allowWorkloads: true`; `restart` also
requires `allowRestart: true`. The test plan cleans up its own claims, pods,
and templates between scenarios. It does not delete cluster-scoped
DeviceClasses or ResourceSlices.

Use the [DRA test-plan section of the user guide](docs/user-guide.md#dra-test-plans)
for topology syntax, lifecycle permissions, verifier scripts, evidence
directories, and the complete scenario behavior.

## Configuration essentials

Configuration is strict YAML: unknown fields are rejected. A driver selects
exactly one artifact source:

- `sourcePath` builds and pushes the checkout image. `registry` is required;
- `image` uses an already-published image with an explicit tag, and `chart` is
  also required; and
- `namespace` optionally selects the driver namespace. Otherwise the harness
  creates a run-owned namespace.

Other frequently used fields include:

- `cleanup`: defaults to `true` for installation runs;
- `upstreamTests`: an argv list, not a shell command, run from the checkout
  after deployment;
- `drivers[].claimConfig`: optional opaque configuration applied to that
  driver's regular pod workload claims, useful for drivers such as SR-IOV
  `STANDALONE` mode;
- `workload: kubevirt` and `kubevirt`: direct-VMI settings;
- `amdOperator` or `nvidiaOperator`: optional operator prerequisites; and
- `sriovPolicy`: an optional raw `SriovResourcePolicy.spec` for the `sriov`
  adapter. The harness creates it after the driver chart and removes it during
  cleanup. If `namespace` is omitted, it uses the run-owned driver namespace.
  If it is set while the driver namespace is omitted, it also selects that
  namespace for the driver; otherwise it must match the configured driver namespace;
  and
- `testPlan.selector` supplies a default CEL selector for regular pod claims,
  while `testPlan.claimConfig` adds optional opaque device configuration to
  those claims; and
- `testPlan`: reusable DRA allocation scenarios.

Relative `sourcePath`, `scriptsDir`, and `evidenceDir` values are resolved
relative to the YAML file. Local chart paths should use a `.`-prefixed path,
such as `./charts/driver`. The harness uses the current `KUBECONFIG`; kubeconfig
is not configured in YAML.
See the [user guide](docs/user-guide.md#run-configuration) for the full
configuration contract and safety behavior.

## Test runner and diagnostics

The Ginkgo runner selects suites under `tests/` with `TEST_FEATURES`:

```sh
# DRA suite only
TEST_FEATURES=dra DRA_HARNESS_CONFIG=examples/example.yaml make run-tests

# Smoke and DRA suites
TEST_FEATURES="smoke dra" DRA_HARNESS_CONFIG=examples/example.yaml make run-tests
```

Useful environment variables:

- `TEST_VERBOSE=true`: verbose Ginkgo output;
- `TEST_TRACE=true`: include Ginkgo traces;
- `TEST_LABELS=...`: apply a Ginkgo label filter; and
- `ARTIFACT_DIR=/path/to/artifacts`: write the smoke JUnit report there.

For failures, first confirm the kubeconfig context and DRA API discovery, then
inspect the driver pods, DeviceClasses, ResourceSlices, ResourceClaims, and
events in the run-specific namespace. Do not delete broad namespaces or all
claims on a shared cluster. See the [troubleshooting guide](docs/user-guide.md#troubleshooting)
for focused checks and cleanup guidance.

## Repository layout

```text
internal/driver/       Built-in adapter contracts and driver implementations
internal/harness/      Cluster lifecycle, readiness, workloads, and cleanup
internal/kubevirt/      Direct-VMI and guest-verification backend
internal/runconfig/    Strict YAML configuration and validation
internal/testplan/     Reusable namespace-scoped DRA scenarios
internal/verification/  External verifier execution and evidence collection
tests/dra/              Integration suite for selected driver runs
tests/smoke/            Basic harness/cluster smoke suite
examples/               Starter configurations for supported workflows
docs/                   User and adapter-authoring documentation
```

## Development

Run the local checks from the repository root:

```sh
make unit-test
make verify
go test -tags=integration ./tests/dra -run '^$'
```

The last command compiles the integration suite without connecting to a
cluster. To add a built-in adapter, implement the
[`internal/driver.Adapter`](internal/driver/driver.go) contract, register the
adapter, and add it to the supplied adapter set. See
[adapter authoring](docs/adapter-authoring.md).

## Documentation

- [User guide](docs/user-guide.md) — complete setup, configuration, safety,
  KubeVirt, operators, test plans, verification, and troubleshooting.
- [Adapter authoring](docs/adapter-authoring.md) — extend the built-in driver
  model.
- [Development TODOs](TODO.md) — current hardware validation and follow-up
  work.

## Current limitations

- A live cluster is required for integration runs; this repository does not
  provision one.
- The `example` adapter is the only path that requires no special hardware;
  CPU runs still depend on CPU runtime configuration.
- AMD, CPU, and NVIDIA runs depend on the selected driver revision and node
  runtime configuration.
- SR-IOV runs require matching hardware and a resource policy; a policy with
  an empty `configs` list will not advertise devices.
- NVIDIA GPU allocation is experimental and should be validated with pinned
  Operator and driver revisions.
- KubeVirt testing depends on an existing KubeVirt/OpenShift Virtualization
  installation and enabled DRA feature gates.
