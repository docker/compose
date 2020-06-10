GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
PWD = $(shell pwd)

export DOCKER_BUILDKIT=1

.DEFAULT_GOAL := build

build: ## Build for the current
	@docker build . \
		--output type=local,dest=./dist \
		--build-arg TARGET_OS=${GOOS} \
		--build-arg TARGET_ARCH=${GOARCH} \
		--target build

cross: ## Cross build for linux, macos and windows
	@docker build . \
		--output type=local,dest=./dist \
		--target cross

test: build ## Run tests
	@docker build . \
		--output type=local,dest=./dist \
		--target test

e2e: build ## Run tests
	go test ./... -v -tags=e2e

dev: build
	@mkdir -p ~/.docker/cli-plugins/
	ln -f -s "${PWD}/dist/docker-ecs" "${HOME}/.docker/cli-plugins/docker-ecs"

lint: ## Verify Go files
	@docker run --rm -t \
		-v $(PWD):/app \
		-w /app \
		golangci/golangci-lint:v1.27-alpine \
		golangci-lint run --timeout 10m0s --config ./golangci.yaml ./...

clean:
	rm -rf dist/

.PHONY: clean build test dev lint e2e cross
