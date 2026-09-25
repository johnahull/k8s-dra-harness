export GO111MODULE=on
MODULE := github.com/johnahull/amd-ci
GO_PACKAGES = $(shell go list ./... | grep -v /vendor/)
# Packages under tests/ are Ginkgo suites that need a cluster; unit-test skips them.
TEST ?= ...
ARGS ?=

.PHONY: help vet lint verify deps-update unit-test install-ginkgo run-tests

help: ## Show available make targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

vet: ## Run go vet
	go vet $(GO_PACKAGES)

lint: ## Run golangci-lint (installs v2.8.0 if needed)
	scripts/golangci-lint.sh

verify: lint vet ## Lint + vet

deps-update: ## go mod tidy && go mod vendor
	go mod tidy && go mod vendor

unit-test: ## Run unit tests (TEST=pkg/amdgpu to narrow)
	go test $$(go list $(MODULE)/$(TEST) | grep -v '/tests/')

install-ginkgo: ## Install the ginkgo CLI
	go install github.com/onsi/ginkgo/v2/ginkgo@v2.28.1

run-tests: ## Run Ginkgo suites (TEST_FEATURES required)
	scripts/test-runner.sh $(ARGS)
