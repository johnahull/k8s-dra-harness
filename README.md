# k8s-dra-harness

Test custom Dynamic Resource Allocation (DRA) drivers on an existing Kubernetes
or OpenShift cluster. The harness can build a local driver checkout, push its
image to a registry, deploy the matching Helm chart, run that checkout's own
end-to-end suite, and then run independent live-workload checks. AMD GPU, NVIDIA
GPU, and CPU are optional, built-in adapters: select one or more in `drivers`,
or select neither for an operator-only run. The `example` adapter targets the
Kubernetes mock-device DRA example driver and does not require GPU hardware.
Additional drivers can follow the
`internal/driver.Adapter` contract. The adapters are compiled into the harness;
they are not separately loaded plugins. To add a built-in driver, add a package
under `internal/driver/` that registers its adapter, then import that package
from `internal/adapters/defaults` for the supplied test suite. A custom Go
entry point can import its own adapter set. An adapter's live workload runs on its
own; a combined workload runs only for pairs with a registered joint policy.

The cluster must serve `resource.k8s.io/v1`, be reachable through `KUBECONFIG`,
and be able to pull any image built for the run. Source builds need Docker,
Helm, a registry login, and the upstream checkout's build dependencies.
OpenShift runs also need `oc`; AMD Operator bundles need `operator-sdk` and OLM.
CPU workloads need the runtime's NRI and CDI support and compatible CPUManager
settings. AMD workloads need GPU nodes with a working amdgpu kernel driver.
NVIDIA runs need the NVIDIA GPU Operator chart and a working NVIDIA
driver/toolkit on the target nodes.

For step-by-step usage, shared-cluster safety guidance, troubleshooting, and
configuration guidance, see the [user guide](docs/user-guide.md). The user
guide is the canonical reference for shared-cluster modes and DRA test-plan
behavior.

KubeVirt runs additionally need an installed KubeVirt or OpenShift
Virtualization deployment with Kubernetes DRA enabled and the matching
`GPUsWithDRA` or `HostDevicesWithDRA` feature gate. The harness does not install
or enable KubeVirt. Select `workload: kubevirt` to create direct VMIs; the
default `workload: pod` path is unchanged. KubeVirt workloads currently
support the AMD and NVIDIA adapters, while CPU remains pod-only. Use
`kubevirt-nvidia.yaml` as a starting point. Its guest verification requires
`virtctl`, a cloud-init Secret that installs the matching public key, and a
private-key Secret containing the configured key.

## Run

```sh
make unit-test
DRA_HARNESS_CONFIG=examples/amd-cpu.yaml TEST_FEATURES=dra make run-tests
```

`DRA_HARNESS_CONFIG` is a strict YAML file. `sourcePath` and local `chart`
paths are relative to the YAML file. Each driver needs either `sourcePath`
or a tagged `image`; an image source also needs `chart`. Source builds use
the checkout chart by default and push a unique image tag under `registry`.
The harness fails before deployment if the target DeviceClass already exists.
It removes only releases and namespaces owned by the run. If Helm cannot confirm
a release's state during cleanup, the harness reports the error and retains its
namespace for inspection. Ownership is not persisted for a later cleanup-only
retry, so resolve the Helm/API problem and inspect the retained release and
namespace manually before another run. Set `cleanup: false` to inspect them
intentionally afterward.
For local charts with dependencies, it runs `helm dependency build` in the
checkout before installing.

For a driver already installed on a shared cluster, set `existing: true` and
provide each driver's existing `namespace`. Without a `testPlan`, this is a
read-only DRA validation: it checks the GA DRA API, DeviceClass, and nonempty
ResourceSlices, and skips installation, workloads, upstream commands, and
cleanup. An existing-driver run with a `testPlan` can create a temporary
workload namespace and mutate claims, pods, or driver pods when its explicit
lifecycle permissions allow those actions.

For a prospective installation, set `preflight: true` with the normal driver
`image`/`chart` or `sourcePath` configuration. Preflight checks cluster/API
availability, DeviceClass collisions, and release-name availability, then
stops without building, installing, or creating workloads.

```yaml
namespace: dra-harness
registry: quay.io/my-account
drivers:
  - name: cpu
    sourcePath: /path/to/dra-driver-cpu
    upstreamTests: [make, test-e2e]
  - name: amd
    sourcePath: /path/to/k8s-gpu-dra-driver
```

`upstreamTests` is an optional argument list executed from that checkout after
deployment. It inherits `KUBECONFIG` and receives `DRA_HARNESS_IMAGE` and
`DRA_HARNESS_NAMESPACE`. Upstream suites may need their own test images or
settings; supply those through their documented environment variables. The
AMD checkout's current `test-e2e` Make target references absent scripts, so
configure this only after confirming the command in the selected revision.

An optional `amdOperator` block installs a supplied AMD GPU Operator chart on
Kubernetes or a bundle image on OpenShift before the selected DRA drivers.
For bundles, set `package` so cleanup can call `operator-sdk cleanup` safely.
For charts, `image` overrides the operator controller image. The block accepts
Helm `values` for the operator's dependencies and DeviceConfig settings.
An operator-only run (no `drivers`) waits for `amd.com/gpu` on a node and runs
a ROCm workload through the device plugin. When combined with a standalone AMD
DRA driver, the harness disables the chart's default device plugin to avoid
competing for the same GPUs.
See [the operator example](examples/amd-operator.yaml). For an OpenShift bundle,
provide `bundle`, `package`, and a `deviceConfig` map containing the raw
`DeviceConfig.spec` fields to create after OLM installs the operator.
Bundle cleanup leaves shared CRDs in place; it removes the run's DeviceConfig,
OLM installation, and namespace.

The `nvidia` adapter builds and installs the NVIDIA DRA driver's
`deployments/helm/dra-driver-nvidia-gpu` chart. An optional `nvidiaOperator`
block installs the NVIDIA GPU Operator chart first. The harness uses the
operator's classic `ClusterPolicy`; when the standalone `nvidia` driver is
selected, it disables the operator's legacy device plugin so only the DRA
driver allocates GPUs. It also waits for `ClusterPolicy` readiness and aligns
the DRA driver's `nvidiaDriverRoot` with the operator's configured driver
directory. The operator's `GPUCluster` DRA mode is intentionally rejected
because it would compete with the standalone driver, and preflight refuses a
pre-existing standard NVIDIA device plugin advertising `nvidia.com/gpu`. See
[the combined NVIDIA example](examples/nvidia.yaml) and
[the operator-only example](examples/nvidia-operator.yaml).

NVIDIA GPU allocation is experimental in the checked-out upstream driver.
Pin the NVIDIA Operator and DRA driver checkouts or image/chart revisions when
using this in repeatable CI, and treat the live NVIDIA run as hardware-specific
validation rather than a production support guarantee.

### DRA test plans

The optional `testPlan` block runs namespace-scoped DRA allocation scenarios
after driver setup. It is also usable with `existing: true`, but claim,
workload, and driver-restart mutations require explicit lifecycle opt-in.

```yaml
existing: true
drivers:
  - name: amd
    namespace: kube-amd-gpu
    podSelector: app.kubernetes.io/name=k8s-gpu-dra-driver
testPlan:
  profile: amd-pr-91-topology
  scenarios: [resource-slices, counters, sibling-exclusion, release, topology]
  lifecycle:
    allowWorkloads: true
  verification:
    scriptsDir: /home/jhull/devel/dra-topology-aware-co-placement/testing/scripts
    expectedRepoCommit: <topology-repository-commit>
    evidenceDir: /tmp/dra-harness-evidence
  topology:
    - name: gpu-cpu-numa
      matchAttribute: resource.kubernetes.io/numaNode
      requests:
        - name: gpu
          deviceClass: gpu.amd.com
        - name: cpu
          deviceClass: dra.cpu
```

The harness records ResourceSlice and ResourceClaim JSON snapshots and invokes
the topology repository's verifier tools when `scriptsDir` is configured. With
the default commands, `dra-verify.sh` and `show-dra-topology.sh` run for the
applicable scenarios, while `dra-counters.py` runs for the `counters`
scenario. Custom `verification.commands` replace the default verifier list.
The verifier checkout commit is recorded with the evidence and can be pinned
with `expectedRepoCommit`. The topology demo script is not invoked directly
because its broad cleanup is unsafe on shared namespaces; its claim cases are
implemented by the harness instead.

See [development TODOs](TODO.md),
[the design](docs/superpowers/specs/2026-09-25-k8s-dra-harness-design.md)
and [the AMD foundation history](docs/superpowers/plans/2026-09-25-amd-ci-foundation.md).
