# Copyright (c) 2020 Docker Inc.

# Permission is hereby granted, free of charge, to any person
# obtaining a copy of this software and associated documentation
# files (the "Software"), to deal in the Software without
# restriction, including without limitation the rights to use, copy,
# modify, merge, publish, distribute, sublicense, and/or sell copies
# of the Software, and to permit persons to whom the Software is
# furnished to do so, subject to the following conditions:

# The above copyright notice and this permission notice shall be
# included in all copies or substantial portions of the Software.

# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
# EXPRESS OR IMPLIED,
# INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
# FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
# IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT
# HOLDERS BE LIABLE FOR ANY CLAIM,
# DAMAGES OR OTHER LIABILITY,
# WHETHER IN AN ACTION OF CONTRACT,
# TORT OR OTHERWISE,
# ARISING FROM, OUT OF OR IN CONNECTION WITH
# THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

export DOCKER_BUILDKIT=1

all: cli

protos: ## Generate go code from .proto files
	@docker build . \
	--output type=local,dest=. \
	--target protos

cli: ## Compile the cli
	@docker build . \
	--output type=local,dest=./bin \
	--build-arg TARGET_OS=${GOOS} \
	--build-arg TARGET_ARCH=${GOARCH} \
	--target cli

e2e-local: ## Run End to end local tests
	go run ./tests/e2e/e2e.go

e2e-aci: ## Run End to end ACI tests (requires azure login)
	go run ./tests/aci-e2e/e2e-aci.go

cross: ## Compile the CLI for linux, darwin and windows
	@docker build . \
	--output type=local,dest=./bin \
	--target cross

test: ## Run unit tests
	@docker build . \
	--target test

cache-clear: ## Clear the builder cache
	@docker builder prune --force --filter type=exec.cachemount --filter=unused-for=24h

lint: ## run linter(s)
	@docker build . \
	--target lint

help: ## Show help
	@echo Please specify a build target. The choices are:
	@grep -E '^[0-9a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

FORCE:

.PHONY: all protos cli e2e-local cross test cache-clear lint help
