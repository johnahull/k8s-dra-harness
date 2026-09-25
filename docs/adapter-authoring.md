# Adapter authoring

Built-in adapters implement `internal/driver.Adapter` and are registered by
an `init` function. The default test suite imports them from
`internal/adapters/defaults`.

An adapter owns only driver-specific behavior:

- `Name`, `DriverName`, and `DeviceClass` identify the driver.
- `ChartDir` identifies the chart inside a source checkout.
- `Image` derives the tagged image used for a source build.
- `Build` builds and pushes a checkout image.
- `Values` merges the image and driver-specific Helm values.
- `Workload` returns a container that proves a claimed device reached the pod.

The generic harness owns cluster clients, Helm releases, namespaces, DRA
claims, readiness checks, and cleanup. An adapter should not create Kubernetes
objects directly from `Build`, `Values`, or `Workload`.

The `example` adapter is a hardware-free reference implementation. It uses the
[Kubernetes DRA example driver](https://github.com/kubernetes-sigs/dra-example-driver),
which advertises mock GPUs as `gpu.example.com` and exposes `GPU_DEVICE_*`
environment variables to allocated containers. Its source checkout build uses
the upstream `deployments/container/Makefile` and its `push` target.

To add another built-in adapter:

1. Add `internal/driver/<name>/<name>.go` and implement `driver.Adapter`.
2. Register it in `init` and import it from `internal/adapters/defaults`.
3. Add adapter unit tests for metadata, image/chart behavior, values, and the
   workload command.
4. Add a run configuration example and document any hardware/runtime
   prerequisites.
5. Register a joint policy only when a combined workload has meaningful
   semantics for the driver pair.
