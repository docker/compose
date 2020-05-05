clean:
	rm -rf dist/

build:
	go build -v -o dist/docker-ecs cmd/main/main.go

test: build ## Run tests
	go test ./... -v

dev: build
	ln -f -s "${PWD}/dist/docker-ecs" "${HOME}/.docker/cli-plugins/docker-ecs"

lint: ## Verify Go files
	golangci-lint run --config ./golangci.yaml ./...

.PHONY: clean build test dev lint
