# Development TODOs

The first priority is a live run of the existing harness. Unit tests, lint, and
vet pass, but the smoke and DRA suites have not been run against a live cluster.

## 1. Validate the current path on a cluster

- [x] Run the smoke suite with a valid `KUBECONFIG` on an existing Kubernetes or
      OpenShift cluster that serves `resource.k8s.io/v1`.
- [x] Add a read-only existing-driver validation path for shared clusters.
- [x] Add a non-mutating preflight path for prospective driver installs.
- [ ] Run one driver from a tagged image and chart, then from a source checkout.
      Record the cluster version, driver revision, config, results, and cleanup
      behavior. Use a disposable namespace and verify that resources owned by
      the run are removed.
- [ ] Run the AMD workload on GPU hardware and the CPU workload where the
      required NRI/CDI support is available. Record prerequisites and any
      adapter-specific failures.

## 2. Prove extensibility with another DRA driver

- [ ] Add a third adapter, preferably for a driver with a mock-device mode, to
      test the adapter contract without requiring another hardware type.
- [ ] Verify that its individual workload runs without changing generic
      harness or run-config logic. Register a joint workload only if the pair
      has a meaningful combined check.
- [ ] Document the adapter authoring steps and the limitations found while
      adding it.

## 3. Decide how adapters are distributed

- [ ] Decide whether compiling adapter packages into the harness is sufficient
      or whether driver repositories must supply adapters independently.
- [ ] If independent adapters are required, design a stable extension contract
      and loading mechanism before implementing one. Keep the existing built-in
      adapters available as examples.

## 4. Generalize builds and upstream test setup

- [ ] Exercise `upstreamTests` with a pinned driver checkout on an existing
      cluster. Document its test image, environment, command, and report needs.
- [ ] Support driver-specific build and push steps where the current AMD and CPU
      recipes do not fit, while preserving tagged images and early validation.
- [ ] Capture upstream test output and exit status alongside harness results.

## 5. Make live runs diagnosable and repeatable

- [ ] Save useful failure artifacts: run config, relevant pod events and logs,
      ResourceClaims, ResourceSlices, DeviceClasses, and Helm release status.
- [ ] Exercise partial installs and failed cleanup on a live cluster; confirm
      that a Helm lookup error retains resources for a retry.
- [ ] After the manual path is reliable, add a CI matrix for supported
      Kubernetes and OpenShift versions, pre-release driver images, and hardware
      pools. Keep hardware-free checks separate from GPU-specific checks.
- [ ] Reconcile the older AMD-only `internal/config`, `internal/inittools`,
      `internal/platform`, and `pkg/amdgpu` foundation packages with the active
      harness path. Retire unused paths only after their intended use is clear.
