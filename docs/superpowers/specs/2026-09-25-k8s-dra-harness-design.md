# k8s-dra-harness design

**Status:** implementation in progress. Supersedes the AMD-only
[draft](2026-09-25-amd-ci-design.md).

The harness targets an existing Kubernetes or OpenShift cluster with
`resource.k8s.io/v1`. The user selects one or more drivers in a YAML file.
Each driver entry supplies a tagged image and chart, or a local checkout.
The checkout path selects the adapter's build recipe and chart. The harness
pushes a unique image to the configured registry and installs that image.

The Go `driver.Adapter` boundary owns the upstream build recipe, chart path,
driver and DeviceClass names, and live workload probe. Generic code connects
to the cluster, rejects installations that would replace an existing
DeviceClass, installs Helm releases, checks pods and ResourceSlices, manages
claims, and cleans up resources owned by the run. The first adapters are AMD
GPU and CPU. A joint test allocates both claims to one pod.

After deployment, an optional `upstreamTests` command runs inside each source
checkout against the same cluster. Harness-owned tests then check DRA objects,
allocation, workload output, and release. Driver-specific upstream test
prerequisites remain the responsibility of that checkout's command.

An optional AMD GPU Operator block accepts a supplied Helm chart and controller
image or an OpenShift bundle image. It installs before DRA drivers. The bundle
package name is required so `operator-sdk cleanup` can remove it. An
operator-only run checks the device plugin and a ROCm workload.

Automated unit, lint, vet, and compile checks run without a cluster. Live tests
need a compatible cluster and hardware. Other adapters, including NVIDIA GPU,
SR-IOV and DRANet, are follow-up work.
