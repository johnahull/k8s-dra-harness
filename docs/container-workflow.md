# Container workflow

The harness container is a client-side test runner. It connects to the
cluster through the caller's kubeconfig, runs the Go/Ginkgo suites, invokes
configured verification commands, and writes evidence to a mounted directory.
It is not a controller and does not need to remain installed in the cluster.

The image includes Go, Ginkgo, kubectl, Helm, the OpenShift client, Git, and
the tools needed by the harness. For KubeVirt guest verification, mount the
virtctl binary at /usr/local/bin/virtctl or build a derived image containing
the matching virtctl version.

## Build and publish

Build with Podman:

~~~sh
make container-build
~~~

Use Docker instead:

~~~sh
CONTAINER_TOOL=docker make container-build
~~~

The default image is
quay.io/johnahull/k8s-dra-harness:<git-short-sha>. Override the registry and
tag when publishing:

~~~sh
IMAGE=registry.example.com/testing/k8s-dra-harness \
IMAGE_TAG=2026-09-30 \
make container-build

IMAGE=registry.example.com/testing/k8s-dra-harness \
IMAGE_TAG=2026-09-30 \
make container-push
~~~

The image build runs the unit tests. Driver images and charts are still built
and published separately, using the image and chart settings in the run YAML.

## Run against Kubernetes

Prepare a run YAML on the client. It can be a local file and does not need to
be added to this repository. If topology or other external verification is
configured, check out that repository separately and reference its
container-visible path in the YAML.

~~~sh
mkdir -p results

podman run --rm --network=host \
  -v "$KUBECONFIG:/run/secrets/kubeconfig:ro" \
  -v "$PWD/run.yaml:/run/config/run.yaml:ro" \
  -v "$PWD/topology-repo:/run/topology:ro" \
  -v "$PWD/results:/results" \
  -e KUBECONFIG=/run/secrets/kubeconfig \
  -e DRA_HARNESS_CONFIG=/run/config/run.yaml \
  -e TEST_FEATURES=dra \
  -e ARTIFACT_DIR=/results \
  quay.io/johnahull/k8s-dra-harness:2026-09-30 run
~~~

The YAML must use paths inside the container. For example:

~~~yaml
testPlan:
  verification:
    scriptsDir: /run/topology/testing/scripts
    evidenceDir: /results/verification
    expectedRepoCommit: 0123456789abcdef0123456789abcdef01234567
    commands:
      - [dra-verify.sh, topology]
      - [dra-verify.sh, counters]
~~~

If the cluster endpoint is not reachable through host networking, omit
--network=host and use the normal container network. On SELinux systems,
Podman may require the :Z volume suffix for client-owned files.

Use shell to inspect the image or verify tool versions:

~~~sh
podman run --rm -it quay.io/johnahull/k8s-dra-harness:2026-09-30 shell
~~~

## Run against OpenShift

Authenticate on the client, export the resulting kubeconfig, and use the same
container command:

~~~sh
oc login https://api.cluster.example.com:6443
export KUBECONFIG="$PWD/kubeconfig"
oc whoami
podman run --rm --network=host \
  -v "$KUBECONFIG:/run/secrets/kubeconfig:ro" \
  -v "$PWD/run.yaml:/run/config/run.yaml:ro" \
  -v "$PWD/results:/results" \
  -e KUBECONFIG=/run/secrets/kubeconfig \
  -e DRA_HARNESS_CONFIG=/run/config/run.yaml \
  -e TEST_FEATURES=dra \
  -e ARTIFACT_DIR=/results \
  quay.io/johnahull/k8s-dra-harness:2026-09-30 run
~~~

The caller's OpenShift identity supplies the permissions used by the harness.
Driver installation and SCC changes therefore require the same privileges as
the equivalent non-container run.
