# Development TODOs

The first priority is a live run of the existing harness. Unit tests, lint, and
vet pass; the tagged hardware-free DRA path now runs successfully, while the
smoke suite has not yet been run against a live cluster.

## 1. Validate the current path on a cluster

- [x] Run the smoke suite with a valid `KUBECONFIG` on an existing Kubernetes or
      OpenShift cluster that serves `resource.k8s.io/v1`.
- [x] Add a read-only existing-driver validation path for shared clusters.
- [x] Add a non-mutating preflight path for prospective driver installs.
- [x] Run the hardware-free example from the tagged `v0.5.0` image and chart on
      OpenShift 4.21.25/Kubernetes 1.34.9. The DRA suite passed 3 specs and
      skipped 2 in a disposable namespace; cleanup removed the Helm release and
      run namespaces.
- [x] Repeat the tagged validation from the pinned `v0.5.0` source checkout
      (`93ff610f3c3bfd603c592d1c281a236c1463edb`). An OpenShift binary Docker
      BuildConfig built with `GO_VERSION=1.26.8`, `BASE_IMAGE=docker.io/ubuntu:22.04`,
      `GOMAXPROCS=2`, and `GOFLAGS=-p=2`, then pushed the image to the internal
      registry at digest `sha256:b0e40fd079e09df0a2d37273e6f6fc98d2a051269eb12c3ed8acc0db11e13d84`.
      The DRA suite passed 3 specs and skipped 2; the temporary build project,
      SCC grant, Helm release, and run resources were removed.
- [ ] Run the AMD workload on GPU hardware and the CPU workload where the
      required NRI/CDI support is available. Record prerequisites and any
      adapter-specific failures.
- [ ] Run the NVIDIA GPU Operator plus standalone NVIDIA DRA driver on a
      disposable NVIDIA cluster. Verify the operator's legacy device plugin is
      disabled, GPU ResourceSlices are published, the DRA workload succeeds,
      and cleanup removes only this run's resources.

## 2. Prove extensibility with another DRA driver

- [x] Add a third adapter, preferably for a driver with a mock-device mode, to
      test the adapter contract without requiring another hardware type.
- [ ] Verify that its individual workload runs without changing generic
      harness or run-config logic. Register a joint workload only if the pair
      has a meaningful combined check.
- [x] Document the adapter authoring steps and the limitations found while
      adding it.

## 3. Decide how adapters are distributed

- [x] Decide whether compiling adapter packages into the harness is sufficient
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

## 6. KubeVirt and OpenShift Virtualization

- [x] Add a direct-VMI KubeVirt workload backend targeting the upstream DRA
      API shape used by KubeVirt main.
- [x] Add KubeVirt API/feature-gate discovery without installing or modifying
      KubeVirt or OpenShift Virtualization.
- [x] Add optional Secret-backed guest verification through `virtctl ssh`.
- [ ] Run the KubeVirt workload on an upstream main cluster with AMD and
      NVIDIA hardware, then repeat on a compatible OpenShift Virtualization
      cluster.
- [ ] Add guest image/cloud-init fixtures and capture VMI, virt-launcher, and
      ResourceClaim artifacts for failed runs.
