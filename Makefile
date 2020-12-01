TAG = "docker-compose:alpine-$(shell git rev-parse --short HEAD)"
GIT_VOLUME = "--volume=$(shell pwd)/.git:/code/.git"

DOCKERFILE ?="Dockerfile"
DOCKER_BUILD_TARGET ?="build"

UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Linux)
	BUILD_SCRIPT = linux
endif
ifeq ($(UNAME_S),Darwin)
	BUILD_SCRIPT = osx
endif

COMPOSE_SPEC_SCHEMA_PATH = "compose/config/compose_spec.json"
COMPOSE_SPEC_RAW_URL = "https://raw.githubusercontent.com/compose-spec/compose-spec/master/schema/compose-spec.json"

all: cli

cli: download-compose-spec ## Compile the cli
	./script/build/$(BUILD_SCRIPT)

download-compose-spec: ## Download the compose-spec schema from it's repo
	curl -so $(COMPOSE_SPEC_SCHEMA_PATH) $(COMPOSE_SPEC_RAW_URL)

cache-clear: ## Clear the builder cache
	@docker builder prune --force --filter type=exec.cachemount --filter=unused-for=24h

base-image: ## Builds base image
	docker build -f $(DOCKERFILE) -t $(TAG) --target $(DOCKER_BUILD_TARGET) .

lint: base-image ## Run linter
	docker run --rm \
		--tty \
		$(GIT_VOLUME) \
		$(TAG) \
		tox -e pre-commit

test-unit: base-image ## Run tests
	docker run --rm \
		--tty \
		$(GIT_VOLUME) \
		$(TAG) \
		pytest -v tests/unit/

test: ## Run all tests
	./script/test/default

pre-commit: lint test-unit cli

help: ## Show help
	@echo Please specify a build target. The choices are:
	@grep -E '^[0-9a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

FORCE:

.PHONY: all cli download-compose-spec cache-clear base-image lint test-unit test pre-commit help
