# DRA Harness

The harness verifies selected DRA drivers against an existing Kubernetes or
OpenShift cluster.

## Language

**Run**:
One verification attempt against a cluster with a selected set of drivers and
an optional AMD GPU Operator.

**Driver adapter**:
The harness's description of how to build, deploy, and exercise one DRA driver.
Selecting an adapter in a run does not require selecting any other adapter.

**Joint workload**:
A live workload that exercises a supported pair of selected drivers in one pod.

**AMD GPU Operator prerequisite**:
An optional operator installation that prepares AMD GPU resources for a run, or
is verified by itself without a standalone DRA driver.
