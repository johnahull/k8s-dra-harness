# amd-gpu-e2e

Ginkgo test framework for release and pre-release AMD GPU Operator and DRA
driver builds on OpenShift and Kubernetes. The foundation and smoke suite are
implemented; deployment and workload suites are planned in the
[design](docs/superpowers/specs/2026-09-25-amd-ci-design.md).

    make unit-test                          # no cluster needed
    TEST_FEATURES=smoke make run-tests      # needs KUBECONFIG
