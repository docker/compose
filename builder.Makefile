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

PKG_NAME := github.com/docker/compose/v2

EXTENSION:=
ifeq ($(GOOS),windows)
  EXTENSION:=.exe
endif

STATIC_FLAGS=CGO_ENABLED=0

GIT_TAG?=$(shell git describe --tags --match "v[0-9]*")

LDFLAGS="-s -w -X $(PKG_NAME)/internal.Version=${GIT_TAG}"
GO_BUILD=$(STATIC_FLAGS) go build -trimpath -ldflags=$(LDFLAGS)

COMPOSE_BINARY?=bin/docker-compose
COMPOSE_BINARY_WITH_EXTENSION=$(COMPOSE_BINARY)$(EXTENSION)

WORK_DIR:=$(shell mktemp -d)

TAGS:=
ifdef BUILD_TAGS
  TAGS=-tags $(BUILD_TAGS)
  LINT_TAGS=--build-tags $(BUILD_TAGS)
endif

.PHONY: compose-plugin
compose-plugin:
	GOOS=${GOOS} GOARCH=${GOARCH} $(GO_BUILD) $(TAGS) -o $(COMPOSE_BINARY_WITH_EXTENSION) ./cmd

.PHONY: cross
cross:
	GOOS=linux   GOARCH=amd64 $(GO_BUILD) $(TAGS) -o $(COMPOSE_BINARY)-linux-x86_64 ./cmd
	GOOS=linux   GOARCH=arm64 $(GO_BUILD) $(TAGS) -o $(COMPOSE_BINARY)-linux-aarch64 ./cmd
	GOOS=linux   GOARM=6 GOARCH=arm $(GO_BUILD) $(TAGS) -o $(COMPOSE_BINARY)-linux-armv6 ./cmd
	GOOS=linux   GOARM=7 GOARCH=arm $(GO_BUILD) $(TAGS) -o $(COMPOSE_BINARY)-linux-armv7 ./cmd
	GOOS=linux   GOARCH=s390x $(GO_BUILD) $(TAGS) -o $(COMPOSE_BINARY)-linux-s390x ./cmd
	GOOS=darwin  GOARCH=amd64 $(GO_BUILD) $(TAGS) -o $(COMPOSE_BINARY)-darwin-x86_64 ./cmd
	GOOS=darwin  GOARCH=arm64 $(GO_BUILD) $(TAGS) -o $(COMPOSE_BINARY)-darwin-aarch64 ./cmd
	GOOS=windows GOARCH=amd64 $(GO_BUILD) $(TAGS) -o $(COMPOSE_BINARY)-windows-x86_64.exe ./cmd

.PHONY: test
test:
	go test $(TAGS) -cover $(shell go list  $(TAGS) ./... | grep -vE 'e2e')

.PHONY: lint
lint:
	golangci-lint run $(LINT_TAGS) --timeout 10m0s ./...

.PHONY: check-license-headers
check-license-headers:
	./scripts/validate/fileheader

.PHONY: check-go-mod
check-go-mod:
	./scripts/validate/check-go-mod

.PHONY: yamldocs
yamldocs:
	go run docs/yaml/main/generate.go