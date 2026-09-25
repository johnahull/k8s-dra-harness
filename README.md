# k8s-dra-harness

Test custom Dynamic Resource Allocation (DRA) drivers on an existing Kubernetes
or OpenShift cluster. The harness can build a local driver checkout, push its
image to a registry, deploy the matching Helm chart, run that checkout's own
end-to-end suite, and then run independent live-workload checks. AMD GPU and CPU
are optional, built-in adapters: select either one or both in `drivers`, or
select neither for an AMD Operator-only run. Additional drivers can follow the
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
namespaces for a retry. Set
`cleanup: false` to inspect them afterward.
For local charts with dependencies, it runs `helm dependency build` in the
checkout before installing.

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

See [the design](docs/superpowers/specs/2026-09-25-k8s-dra-harness-design.md)
and [the AMD foundation history](docs/superpowers/plans/2026-09-25-amd-ci-foundation.md).
