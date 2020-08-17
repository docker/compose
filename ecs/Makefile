PLATFORM?=local
PWD=$(shell pwd)

export DOCKER_BUILDKIT=1

COMMIT := $(shell git rev-parse --short HEAD)
TAG := $(shell git describe --tags --dirty --match "v*")

.DEFAULT_GOAL := build

build: ## Build for the current
	@docker build . \
		--output ./dist \
		--platform ${PLATFORM} \
		--build-arg COMMIT=${COMMIT} \
		--build-arg TAG=${TAG} \
		--target build

cross: ## Cross build for linux, macos and windows
	@docker build . \
		--output ./dist \
		--build-arg COMMIT=${COMMIT} \
		--build-arg TAG=${TAG} \
		--target cross

test: build ## Run tests
	@docker build . \
		--build-arg COMMIT=${COMMIT} \
		--build-arg TAG=${TAG} \
        --target test

e2e: build ## Run tests
	go test ./... -v -tags=e2e

dev: build
	@mkdir -p ~/.docker/cli-plugins/
	ln -f -s "${PWD}/dist/docker-ecs" "${HOME}/.docker/cli-plugins/docker-ecs"

lint: ## Verify Go files
	@docker build . --target lint

fmt: ## Format go files
	go fmt ./...

clean:
	rm -rf dist/

.PHONY: clean build test dev lint e2e cross fmt
