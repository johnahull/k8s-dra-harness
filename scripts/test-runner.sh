#!/usr/bin/env bash
# Runs the Ginkgo suites under tests/ selected by TEST_FEATURES (space- or
# comma-separated directory names, or "all") and filtered by TEST_LABELS.

GOPATH="${GOPATH:-${HOME}/go}"
PATH=$PATH:$GOPATH/bin
TEST_DIR="./tests"

# Ginkgo changes its working directory to each suite package. Normalize the
# run config while still at the repository root so documented relative paths
# such as examples/cpu-preflight.yaml continue to work.
if [[ -n "${DRA_HARNESS_CONFIG}" && "${DRA_HARNESS_CONFIG}" != /* ]]; then
    export DRA_HARNESS_CONFIG="$(pwd)/${DRA_HARNESS_CONFIG}"
fi

if [[ -n "${ARTIFACT_DIR}" ]]; then
    export REPORTS_DUMP_DIR=${ARTIFACT_DIR}
fi

if [[ -z "${TEST_FEATURES}" ]]; then
    echo "TEST_FEATURES environment variable is undefined"
    exit 1
fi

if [[ "${TEST_FEATURES}" == "all" ]]; then
    feature_dirs=${TEST_DIR}
else
    for feature in ${TEST_FEATURES//,/ }; do
        discovered=$(find $TEST_DIR -depth -type d -name "${feature}" 2> /dev/null)
        if [[ -n $discovered ]]; then
            feature_dirs+=" "$discovered
        elif [[ "${VERBOSE_SCRIPT}" == "true" ]]; then
            echo "Could not find any feature directories matching ${feature}"
        fi
    done

    if [[ -z "${feature_dirs}" ]]; then
        echo "Could not find any feature directories for provided features: ${TEST_FEATURES}"
        exit 1
    fi
fi

cmd="ginkgo -timeout=24h --keep-going --require-suite -r --tags=integration"

if [[ "${TEST_VERBOSE}" == "true" ]]; then
    cmd+=" -vv"
fi

if [[ "${TEST_TRACE}" == "true" ]]; then
    cmd+=" --trace"
fi

if [[ -n "${TEST_LABELS}" ]]; then
    cmd+=" --label-filter=\"${TEST_LABELS}\""
fi
cmd+=" ${feature_dirs} $*"

echo "$cmd"
eval "$cmd"
