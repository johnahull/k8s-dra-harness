ARG GO_VERSION=1.26
FROM golang:${GO_VERSION}-bookworm

ARG KUBECTL_VERSION=v1.37.1
ARG HELM_VERSION=v3.18.6
ARG OC_VERSION=4.20.0
ARG GINKGO_VERSION=v2.28.1

ENV DEBIAN_FRONTEND=noninteractive \
    PATH=/go/bin:/usr/local/go/bin:$PATH \
    TEST_FEATURES=dra \
    ARTIFACT_DIR=/results

RUN apt-get update \
    && apt-get install --no-install-recommends -y \
        bash \
        ca-certificates \
        curl \
        git \
        gzip \
        make \
        tar

RUN curl -fsSL "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl" \
        -o /usr/local/bin/kubectl \
    && chmod 0755 /usr/local/bin/kubectl \
    && curl -fsSL "https://get.helm.sh/helm-${HELM_VERSION}-linux-amd64.tar.gz" \
        | tar -xz -C /tmp \
    && install -m 0755 /tmp/linux-amd64/helm /usr/local/bin/helm \
    && curl -fsSL "https://mirror.openshift.com/pub/openshift-v4/clients/ocp/${OC_VERSION}/openshift-client-linux.tar.gz" \
        | tar -xz -C /tmp \
    && install -m 0755 /tmp/oc /usr/local/bin/oc

WORKDIR /opt/k8s-dra-harness
COPY . .

RUN go install github.com/onsi/ginkgo/v2/ginkgo@${GINKGO_VERSION} \
    && go test ./... \
    && go test -tags=integration ./tests/dra -run '^$'

COPY scripts/container-entrypoint.sh /usr/local/bin/dra-harness
RUN chmod 0755 /usr/local/bin/dra-harness \
    && mkdir -p /results

ENTRYPOINT ["dra-harness"]
CMD ["help"]
