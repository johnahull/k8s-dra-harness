# amd-ci

Ginkgo test framework for the AMD GPU Operator and AMD DRA driver on
OpenShift and Kubernetes. Design: `docs/superpowers/specs/2026-09-25-amd-ci-design.md`.

    make unit-test                          # no cluster needed
    TEST_FEATURES=smoke make run-tests      # needs KUBECONFIG
