#!/usr/bin/env bash
set -euo pipefail

REPO_DIR=/opt/k8s-dra-harness

usage() {
    cat <<'USAGE'
Usage:
  dra-harness run [CONFIG] [ginkgo arguments...]
  dra-harness shell
  dra-harness help

Environment:
  DRA_HARNESS_CONFIG  YAML configuration path, unless CONFIG is supplied
  TEST_FEATURES       Suite directories to run; defaults to dra
  ARTIFACT_DIR        Evidence and reports directory; defaults to /results
USAGE
}

run_tests() {
    local config=${1:-${DRA_HARNESS_CONFIG:-}}
    if [[ -z "${config}" ]]; then
        echo "a run YAML is required: pass CONFIG or set DRA_HARNESS_CONFIG" >&2
        exit 2
    fi
    shift || true

    if [[ "${config}" != /* ]]; then
        config="${REPO_DIR}/${config}"
    fi
    if [[ ! -f "${config}" ]]; then
        echo "run configuration does not exist: ${config}" >&2
        exit 2
    fi

    export DRA_HARNESS_CONFIG="${config}"
    export TEST_FEATURES="${TEST_FEATURES:-dra}"
    export ARTIFACT_DIR="${ARTIFACT_DIR:-/results}"
    mkdir -p "${ARTIFACT_DIR}"

    cd "${REPO_DIR}"
    exec scripts/test-runner.sh "$@"
}

case "${1:-help}" in
run)
    shift
    run_tests "$@"
    ;;
shell)
    exec bash
    ;;
help|-h|--help)
    usage
    ;;
*)
    echo "unknown command: $1" >&2
    usage >&2
    exit 2
    ;;
esac
