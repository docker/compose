PLATFORM?=local
PWD=$(shell pwd)

export DOCKER_BUILDKIT=1

.DEFAULT_GOAL := build

build: ## Build for the current
	@docker build . \
		--output ./dist \
		--platform ${PLATFORM} \
		--target build

cross: ## Cross build for linux, macos and windows
	@docker build . \
		--output ./dist \
		--target cross

test: build ## Run tests
	@docker build . --target test

e2e: build ## Run tests
	go test ./... -v -tags=e2e

dev: build
	@mkdir -p ~/.docker/cli-plugins/
	ln -f -s "${PWD}/dist/docker-ecs" "${HOME}/.docker/cli-plugins/docker-ecs"

lint: ## Verify Go files
	@docker build . --target lint

clean:
	rm -rf dist/

.PHONY: clean build test dev lint e2e cross
