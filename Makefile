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

export DOCKER_BUILDKIT=1

UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Linux)
	MOBY_DOCKER=/usr/bin/docker
endif
ifeq ($(UNAME_S),Darwin)
	MOBY_DOCKER=/Applications/Docker.app/Contents/Resources/bin/docker
endif

BINARY_FOLDER=$(shell pwd)/bin
GIT_TAG?=$(shell git describe --tags --match "v[0-9]*")
TEST_FLAGS?=
E2E_TEST?=
ifeq ($(E2E_TEST),)
else
	TEST_FLAGS=-run $(E2E_TEST)
endif

all: compose-plugin

.PHONY: compose-plugin
compose-plugin: ## Compile the compose cli-plugin
	@docker build . --target compose-plugin \
	--platform local \
	--build-arg BUILD_TAGS=e2e,kube \
	--build-arg GIT_TAG=$(GIT_TAG) \
	--output ./bin

.PHONY: e2e-compose
e2e-compose: ## Run end to end local tests in plugin mode. Set E2E_TEST=TestName to run a single test
	docker compose version
	go test $(TEST_FLAGS) -count=1 ./pkg/e2e

.PHONY: e2e-compose-standalone
e2e-compose-standalone: ## Run End to end local tests in standalone mode. Set E2E_TEST=TestName to run a single test
	docker-compose version
	go test $(TEST_FLAGS) -v -count=1 -parallel=1 --tags=standalone ./pkg/e2e

.PHONY: build-and-e2e-compose
build-and-e2e-compose: compose-plugin e2e-compose ## Compile the compose cli-plugin and run end to end local tests in plugin mode. Set E2E_TEST=TestName to run a single test

.PHONY: build-and-e2e-compose-standalone
build-and-e2e-compose-standalone: compose-plugin e2e-compose-standalone ## Compile the compose cli-plugin and run End to end local tests in standalone mode. Set E2E_TEST=TestName to run a single test

.PHONY: mocks
mocks:
	mockgen -destination pkg/mocks/mock_docker_cli.go -package mocks github.com/docker/cli/cli/command Cli
	mockgen -destination pkg/mocks/mock_docker_api.go -package mocks github.com/docker/docker/client APIClient
	mockgen -destination pkg/mocks/mock_docker_compose_api.go -package mocks -source=./pkg/api/api.go Service

.PHONY: e2e
e2e: e2e-compose e2e-compose-standalone ## Run end to end local tests in both modes. Set E2E_TEST=TestName to run a single test

.PHONY: build-and-e2e
build-and-e2e: compose-plugin e2e-compose e2e-compose-standalone ## Compile the compose cli-plugin and run end to end local tests in both modes. Set E2E_TEST=TestName to run a single test

.PHONY: cross
cross: ## Compile the CLI for linux, darwin and windows
	@docker build . --target cross \
	--build-arg BUILD_TAGS \
	--build-arg GIT_TAG=$(GIT_TAG) \
	--output ./bin \

.PHONY: test
test: ## Run unit tests
	@docker build --progress=plain . \
	--build-arg BUILD_TAGS=kube \
	--build-arg GIT_TAG=$(GIT_TAG) \
	--target test

.PHONY: cache-clear
cache-clear: ## Clear the builder cache
	@docker builder prune --force --filter type=exec.cachemount --filter=unused-for=24h

.PHONY: lint
lint: ## run linter(s)
	@docker build . \
	--build-arg BUILD_TAGS=kube,e2e \
	--build-arg GIT_TAG=$(GIT_TAG) \
	--target lint

.PHONY: docs
docs: ## generate documentation
	$(eval $@_TMP_OUT := $(shell mktemp -d -t dockercli-output.XXXXXXXXXX))
	docker build . \
	--output type=local,dest=$($@_TMP_OUT) \
	-f ./docs/Dockerfile \
	--target update
	rm -rf ./docs/internal
	cp -R "$($@_TMP_OUT)"/out/* ./docs/
	rm -rf "$($@_TMP_OUT)"/*

.PHONY: validate-docs
validate-docs: ## validate the doc does not change
	@docker build . \
	-f ./docs/Dockerfile \
	--target validate

.PHONY: check-dependencies
check-dependencies: ## check dependency updates
	go list -u -m -f '{{if not .Indirect}}{{if .Update}}{{.}}{{end}}{{end}}' all

.PHONY: validate-headers
validate-headers: ## Check license header for all files
	@docker build . --target check-license-headers

.PHONY: go-mod-tidy
go-mod-tidy: ## Run go mod tidy in a container and output resulting go.mod and go.sum
	@docker build . --target go-mod-tidy --output .

.PHONY: validate-go-mod
validate-go-mod: ## Validate go.mod and go.sum are up-to-date
	@docker build . --target check-go-mod

validate: validate-go-mod validate-headers validate-docs ## Validate sources

pre-commit: validate check-dependencies lint compose-plugin test e2e-compose

help: ## Show help
	@echo Please specify a build target. The choices are:
	@grep -E '^[0-9a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
