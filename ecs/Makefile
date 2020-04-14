build:
	go build -v -o dist/docker-ecs cmd/main/main.go

test: ## Run tests
	go test ./... -v

dev: build
	ln -f -s "${PWD}/dist/docker-ecs" "${HOME}/.docker/cli-plugins/docker-ecs"

.PHONY: build test dev