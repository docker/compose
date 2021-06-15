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

GOOS?=$(shell go env GOOS)
GOARCH?=$(shell go env GOARCH)

PKG_NAME := github.com/docker/compose-cli

PROTOS=$(shell find cli/server/protos -name \*.proto)

EXTENSION:=
ifeq ($(GOOS),windows)
  EXTENSION:=.exe
endif

STATIC_FLAGS=CGO_ENABLED=0

GIT_TAG?=$(shell git describe --tags --match "v[0-9]*")

LDFLAGS="-s -w -X $(PKG_NAME)/internal.Version=${GIT_TAG}"
GO_BUILD=$(STATIC_FLAGS) go build -trimpath -ldflags=$(LDFLAGS)

BINARY?=bin/docker
BINARY_WITH_EXTENSION=$(BINARY)$(EXTENSION)

COMPOSE_BINARY?=bin/docker-compose
COMPOSE_BINARY_WITH_EXTENSION=$(COMPOSE_BINARY)$(EXTENSION)

WORK_DIR:=$(shell mktemp -d)

TAGS:=
ifdef BUILD_TAGS
  TAGS=-tags $(BUILD_TAGS)
  LINT_TAGS=--build-tags $(BUILD_TAGS)
endif

.PHONY: protos
protos:
	protoc -I. --go_out=plugins=grpc,paths=source_relative:. ${PROTOS}

.PHONY: cli
cli:
	GOOS=${GOOS} GOARCH=${GOARCH} $(GO_BUILD) $(TAGS) -o $(BINARY_WITH_EXTENSION) ./cli

.PHONY: compose-plugin
compose-plugin:
	GOOS=${GOOS} GOARCH=${GOARCH} $(GO_BUILD) $(TAGS) -o $(COMPOSE_BINARY_WITH_EXTENSION) ./cmd

.PHONY: cross
cross:
	GOOS=linux   GOARCH=amd64 $(GO_BUILD) $(TAGS) -o $(BINARY)-linux-amd64 ./cli
	GOOS=linux   GOARCH=arm64 $(GO_BUILD) $(TAGS) -o $(BINARY)-linux-arm64 ./cli
	GOOS=linux   GOARM=6 GOARCH=arm $(GO_BUILD) $(TAGS) -o $(BINARY)-linux-armv6 ./cli
	GOOS=linux   GOARM=7 GOARCH=arm $(GO_BUILD) $(TAGS) -o $(BINARY)-linux-armv7 ./cli
	GOOS=darwin  GOARCH=amd64 $(GO_BUILD) $(TAGS) -o $(BINARY)-darwin-amd64 ./cli
	GOOS=darwin  GOARCH=arm64 $(GO_BUILD) $(TAGS) -o $(BINARY)-darwin-arm64 ./cli
	GOOS=windows GOARCH=amd64 $(GO_BUILD) $(TAGS) -o $(BINARY)-windows-amd64.exe ./cli

.PHONY: cross-compose-plugin
cross-compose-plugin:
	GOOS=linux   GOARCH=amd64 $(GO_BUILD) $(TAGS) -o $(COMPOSE_BINARY)-linux-amd64 ./cmd
	GOOS=linux   GOARCH=arm64 $(GO_BUILD) $(TAGS) -o $(COMPOSE_BINARY)-linux-arm64 ./cmd
	GOOS=linux   GOARM=6 GOARCH=arm $(GO_BUILD) $(TAGS) -o $(COMPOSE_BINARY)-linux-armv6 ./cmd
	GOOS=linux   GOARM=7 GOARCH=arm $(GO_BUILD) $(TAGS) -o $(COMPOSE_BINARY)-linux-armv7 ./cmd
	GOOS=darwin  GOARCH=amd64 $(GO_BUILD) $(TAGS) -o $(COMPOSE_BINARY)-darwin-amd64 ./cmd
	GOOS=darwin  GOARCH=arm64 $(GO_BUILD) $(TAGS) -o $(COMPOSE_BINARY)-darwin-arm64 ./cmd
	GOOS=windows GOARCH=amd64 $(GO_BUILD) $(TAGS) -o $(COMPOSE_BINARY)-windows-amd64.exe ./cmd

.PHONY: test
test:
	go test $(TAGS) -cover $(shell go list  $(TAGS) ./... | grep -vE 'e2e')

.PHONY: lint
lint:
	golangci-lint run $(LINT_TAGS) --timeout 10m0s ./...

.PHONY: import-restrictions
import-restrictions:
	import-restrictions --configuration import-restrictions.yaml

.PHONY: check-licese-headers
check-license-headers:
	./scripts/validate/fileheader

.PHONY: check-go-mod
check-go-mod:
	./scripts/validate/check-go-mod

.PHONY: yamldocs
yamldocs:
	go run docs/yaml/main/generate.go