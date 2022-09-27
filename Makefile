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

GO_LDFLAGS ?= -s -w -X ${PKG}/internal.Version=${VERSION}
GO_BUILDTAGS ?= e2e,kube

ifeq ($(OS),Windows_NT)
    DETECTED_OS = Windows
else
    DETECTED_OS = $(shell uname -s)
endif
ifeq ($(DETECTED_OS),Linux)
	MOBY_DOCKER=/usr/bin/docker
endif
ifeq ($(DETECTED_OS),Darwin)
	MOBY_DOCKER=/Applications/Docker.app/Contents/Resources/bin/docker
endif
ifeq ($(DETECTED_OS),Windows)
	BINARY_EXT=.exe
endif

TEST_FLAGS?=
E2E_TEST?=
ifeq ($(E2E_TEST),)
else
	TEST_FLAGS=-run $(E2E_TEST)
endif

BUILDX_CMD ?= docker buildx
DESTDIR ?= ./bin/build

all: build

.PHONY: build ## Build the compose cli-plugin
build:
	CGO_ENABLED=0 GO111MODULE=on go build -trimpath -tags "$(GO_BUILDTAGS)" -ldflags "$(GO_LDFLAGS)" -o "$(DESTDIR)/docker-compose$(BINARY_EXT)" ./cmd

.PHONY: binary
binary:
	$(BUILDX_CMD) bake binary

.PHONY: install
install: binary
	mkdir -p ~/.docker/cli-plugins
	install bin/build/docker-compose ~/.docker/cli-plugins/docker-compose

.PHONY: e2e-compose
e2e-compose: ## Run end to end local tests in plugin mode. Set E2E_TEST=TestName to run a single test
	docker compose version
	go test $(TEST_FLAGS) -count=1 ./pkg/e2e

.PHONY: e2e-compose-standalone
e2e-compose-standalone: ## Run End to end local tests in standalone mode. Set E2E_TEST=TestName to run a single test
	docker-compose version
	go test $(TEST_FLAGS) -v -count=1 -parallel=1 --tags=standalone ./pkg/e2e

.PHONY: build-and-e2e-compose
build-and-e2e-compose: build e2e-compose ## Compile the compose cli-plugin and run end to end local tests in plugin mode. Set E2E_TEST=TestName to run a single test

.PHONY: build-and-e2e-compose-standalone
build-and-e2e-compose-standalone: build e2e-compose-standalone ## Compile the compose cli-plugin and run End to end local tests in standalone mode. Set E2E_TEST=TestName to run a single test

.PHONY: mocks
mocks:
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
	cp -R "$($@_TMP_OUT)"/out/* ./docs/
	rm -rf "$($@_TMP_OUT)"/*

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
	$(BUILDX_CMD) bake modules-validate

.PHONY: validate-modules
validate-modules: ## Validate root and e2e go.mod are synced
	$(BUILDX_CMD) bake modules-validate

validate: validate-go-mod validate-modules validate-headers validate-docs  ## Validate sources

pre-commit: validate check-dependencies lint build test e2e-compose

help: ## Show help
	@echo Please specify a build target. The choices are:
	@grep -E '^[0-9a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
