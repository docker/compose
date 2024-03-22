#   Copyright 2020 Docker Compose CLI authors

#   Licensed under the Apache License, Version 2.0 (the "License");
#   you may not use this file except in compliance with the License.
#   You may obtain a copy of the License at

#       http://www.apache.org/licenses/LICENSE-2.0

#   Unless required by applicable law or agreed to in writing, software
#   distributed under the License is distributed on an "AS IS" BASIS,
#   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#   See the License for the specific language governing permissions and
#   limitations under the License.

PKG := github.com/docker/compose/v2
VERSION ?= $(shell git describe --match 'v[0-9]*' --dirty='.m' --always --tags)

GO_LDFLAGS ?= -w -X ${PKG}/internal.Version=${VERSION}
GO_BUILDTAGS ?= e2e
DRIVE_PREFIX?=
ifeq ($(OS),Windows_NT)
    DETECTED_OS = Windows
    DRIVE_PREFIX=C:
else
    DETECTED_OS = $(shell uname -s)
endif

ifeq ($(DETECTED_OS),Windows)
	BINARY_EXT=.exe
endif

BUILD_FLAGS?=
TEST_FLAGS?=
E2E_TEST?=
ifneq ($(E2E_TEST),)
	TEST_FLAGS:=$(TEST_FLAGS) -run '$(E2E_TEST)'
endif

EXCLUDE_E2E_TESTS?=
ifneq ($(EXCLUDE_E2E_TESTS),)
	TEST_FLAGS:=$(TEST_FLAGS) -skip '$(EXCLUDE_E2E_TESTS)'
endif

BUILDX_CMD ?= docker buildx

# DESTDIR overrides the output path for binaries and other artifacts
# this is used by docker/docker-ce-packaging for the apt/rpm builds,
# so it's important that the resulting binary ends up EXACTLY at the
# path $DESTDIR/docker-compose when specified.
#
# See https://github.com/docker/docker-ce-packaging/blob/e43fbd37e48fde49d907b9195f23b13537521b94/rpm/SPECS/docker-compose-plugin.spec#L47
#
# By default, all artifacts go to subdirectories under ./bin/ in the
# repo root, e.g. ./bin/build, ./bin/coverage, ./bin/release.
DESTDIR ?=

all: build

.PHONY: build ## Build the compose cli-plugin
build:
	GO111MODULE=on go build $(BUILD_FLAGS) -trimpath -tags "$(GO_BUILDTAGS)" -ldflags "$(GO_LDFLAGS)" -o "$(or $(DESTDIR),./bin/build)/docker-compose$(BINARY_EXT)" ./cmd

.PHONY: binary
binary:
	$(BUILDX_CMD) bake binary

.PHONY: binary-with-coverage
binary-with-coverage:
	$(BUILDX_CMD) bake binary-with-coverage

.PHONY: install
install: binary
	mkdir -p ~/.docker/cli-plugins
	install $(or $(DESTDIR),./bin/build)/docker-compose ~/.docker/cli-plugins/docker-compose

.PHONY: e2e-compose
e2e-compose: ## Run end to end local tests in plugin mode. Set E2E_TEST=TestName to run a single test
	go run gotest.tools/gotestsum@latest --format testname --junitfile "/tmp/report/report.xml" -- -v $(TEST_FLAGS) -count=1 ./pkg/e2e

.PHONY: e2e-compose-standalone
e2e-compose-standalone: ## Run End to end local tests in standalone mode. Set E2E_TEST=TestName to run a single test
	go run gotest.tools/gotestsum@latest --format testname --junitfile "/tmp/report/report.xml" -- $(TEST_FLAGS) -v -count=1 -parallel=1 --tags=standalone ./pkg/e2e

.PHONY: build-and-e2e-compose
build-and-e2e-compose: build e2e-compose ## Compile the compose cli-plugin and run end to end local tests in plugin mode. Set E2E_TEST=TestName to run a single test

.PHONY: build-and-e2e-compose-standalone
build-and-e2e-compose-standalone: build e2e-compose-standalone ## Compile the compose cli-plugin and run End to end local tests in standalone mode. Set E2E_TEST=TestName to run a single test

.PHONY: mocks
mocks:
	mockgen --version >/dev/null 2>&1 || go install go.uber.org/mock/mockgen@v0.4.0
	mockgen -destination pkg/mocks/mock_docker_cli.go -package mocks github.com/docker/cli/cli/command Cli
	mockgen -destination pkg/mocks/mock_docker_api.go -package mocks github.com/docker/docker/client APIClient
	mockgen -destination pkg/mocks/mock_docker_compose_api.go -package mocks -source=./pkg/api/api.go Service

.PHONY: e2e
e2e: e2e-compose e2e-compose-standalone ## Run end to end local tests in both modes. Set E2E_TEST=TestName to run a single test

.PHONY: build-and-e2e
build-and-e2e: build e2e-compose e2e-compose-standalone ## Compile the compose cli-plugin and run end to end local tests in both modes. Set E2E_TEST=TestName to run a single test

.PHONY: cross
cross: ## Compile the CLI for linux, darwin and windows
	$(BUILDX_CMD) bake binary-cross

.PHONY: test
test: ## Run unit tests
	$(BUILDX_CMD) bake test

.PHONY: cache-clear
cache-clear: ## Clear the builder cache
	$(BUILDX_CMD) prune --force --filter type=exec.cachemount --filter=unused-for=24h

.PHONY: lint
lint: ## run linter(s)
	$(BUILDX_CMD) bake lint

.PHONY: docs
docs: ## generate documentation
	$(eval $@_TMP_OUT := $(shell mktemp -d -t compose-output.XXXXXXXXXX))
	$(BUILDX_CMD) bake --set "*.output=type=local,dest=$($@_TMP_OUT)" docs-update
	rm -rf ./docs/internal
	cp -R "$(DRIVE_PREFIX)$($@_TMP_OUT)"/out/* ./docs/
	rm -rf "$(DRIVE_PREFIX)$($@_TMP_OUT)"/*

.PHONY: validate-docs
validate-docs: ## validate the doc does not change
	$(BUILDX_CMD) bake docs-validate

.PHONY: check-dependencies
check-dependencies: ## check dependency updates
	go list -u -m -f '{{if not .Indirect}}{{if .Update}}{{.}}{{end}}{{end}}' all

.PHONY: validate-headers
validate-headers: ## Check license header for all files
	$(BUILDX_CMD) bake license-validate

.PHONY: go-mod-tidy
go-mod-tidy: ## Run go mod tidy in a container and output resulting go.mod and go.sum
	$(BUILDX_CMD) bake vendor-update

.PHONY: validate-go-mod
validate-go-mod: ## Validate go.mod and go.sum are up-to-date
	$(BUILDX_CMD) bake vendor-validate

validate: validate-go-mod validate-headers validate-docs  ## Validate sources

pre-commit: validate check-dependencies lint build test e2e-compose

help: ## Show help
	@echo Please specify a build target. The choices are:
	@grep -E '^[0-9a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
