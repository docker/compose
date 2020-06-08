GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

PROTOS=$(shell find . -name \*.proto)

EXTENSION :=
ifeq ($(GOOS),windows)
  EXTENSION := .exe
endif

STATIC_FLAGS= CGO_ENABLED=0
LDFLAGS := "-s -w"
GO_BUILD = $(STATIC_FLAGS) go build -trimpath -ldflags=$(LDFLAGS)

BINARY=dist/docker
BINARY_WITH_EXTENSION=$(BINARY)$(EXTENSION)

export DOCKER_BUILDKIT=1

all: build

clean:
	rm -rf dist/

build:
	$(GO_BUILD) -v -o $(BINARY_WITH_EXTENSION) cmd/main/main.go

cross:
	@GOOS=linux   GOARCH=amd64 $(GO_BUILD) -v -o $(BINARY)-linux-amd64 cmd/main/main.go
	@GOOS=darwin  GOARCH=amd64 $(GO_BUILD) -v -o $(BINARY)-darwin-amd64 cmd/main/main.go
	@GOOS=windows GOARCH=amd64 $(GO_BUILD) -v -o $(BINARY)-windows-amd64.exe cmd/main/main.go

test: build ## Run tests
	@go test ./... -v

lint: ## Verify Go files
	golangci-lint run --timeout 10m0s --config ./golangci.yaml ./...

.PHONY: clean build test dev lint e2e
