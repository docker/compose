#   Copyright 2020 The 2020 Docker, Inc.

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

GIT_TAG?=$(shell git describe --tags --match "v[0-9]*")
TEST_FLAGS?=
E2E_TEST?=
ifeq ($(E2E_TEST),)
else
	TEST_FLAGS=-run $(E2E_TEST)
endif

all: cli

protos: ## Generate go code from .proto files
	@docker build . --target protos \
	--output ./protos

cli: ## Compile the cli
	@docker build . --target cli \
	--platform local \
	--build-arg BUILD_TAGS=example,local,ecs \
	--build-arg GIT_TAG=$(GIT_TAG) \
	--output ./bin

e2e-local: ## Run End to end local tests. Set E2E_TEST=TestName to run a single test
	go test -count=1 -v $(TEST_FLAGS) ./tests/e2e ./tests/skip-win-ci-e2e ./local/e2e

e2e-win-ci: ## Run end to end local tests on Windows CI, no Docker for Linux containers available ATM. Set E2E_TEST=TestName to run a single test
	go test -count=1 -v $(TEST_FLAGS) ./tests/e2e

e2e-aci: ## Run End to end ACI tests. Set E2E_TEST=TestName to run a single test
	go test -count=1 -v $(TEST_FLAGS) ./tests/aci-e2e

cross: ## Compile the CLI for linux, darwin and windows
	@docker build . --target cross \
	--build-arg BUILD_TAGS \
	--build-arg GIT_TAG=$(GIT_TAG) \
	--output ./bin \

test: ## Run unit tests
	@docker build . \
	--build-arg BUILD_TAGS=example,local,ecs \
	--build-arg GIT_TAG=$(GIT_TAG) \
	--target test

cache-clear: ## Clear the builder cache
	@docker builder prune --force --filter type=exec.cachemount --filter=unused-for=24h

lint: ## run linter(s)
	@docker build . \
	--build-arg GIT_TAG=$(GIT_TAG) \
	--target lint

serve: cli ## start server
	@./bin/docker serve --address unix:///tmp/backend.sock

moby-cli-link: ## create com.docker.cli symlink if does not already exist
	ln -s $(MOBY_DOCKER) /usr/local/bin/com.docker.cli

help: ## Show help
	@echo Please specify a build target. The choices are:
	@grep -E '^[0-9a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

FORCE:

.PHONY: all protos cli e2e-local cross test cache-clear lint serve classic-link help
