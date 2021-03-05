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

TAR_TRANSFORM:=--transform s/packaging/docker/ --transform s/bin/docker/ --transform s/docker-linux-amd64/docker/ --transform s/docker-darwin-amd64/docker/ --transform s/docker-linux-arm64/docker/ --transform s/docker-linux-armv6/docker/ --transform s/docker-linux-armv7/docker/ --transform s/docker-darwin-arm64/docker/
ifneq ($(findstring bsd,$(shell tar --version)),)
  TAR_TRANSFORM=-s /packaging/docker/ -s /bin/docker/ -s /docker-linux-amd64/docker/ -s /docker-darwin-amd64/docker/ -s /docker-linux-arm64/docker/ -s /docker-linux-armv6/docker/ -s /docker-linux-armv7/docker/ -s /docker-darwin-arm64/docker/
endif

all: cli

.PHONY: protos
protos:
	protoc -I. --go_out=plugins=grpc,paths=source_relative:. ${PROTOS}

.PHONY: cli
cli: compose-plugin
	GOOS=${GOOS} GOARCH=${GOARCH} $(GO_BUILD) $(TAGS) -o $(BINARY_WITH_EXTENSION) ./cli

.PHONY: compose-plugin
compose-plugin:
	GOOS=${GOOS} GOARCH=${GOARCH} $(GO_BUILD) $(TAGS) -o $(COMPOSE_BINARY_WITH_EXTENSION) ./compose_plugin

.PHONY: cross
cross:
	GOOS=linux   GOARCH=amd64 $(GO_BUILD) $(TAGS) -o $(BINARY)-linux-amd64 ./cli
	GOOS=linux   GOARCH=arm64 $(GO_BUILD) $(TAGS) -o $(BINARY)-linux-arm64 ./cli
	GOOS=linux   GOARM=6 GOARCH=arm $(GO_BUILD) $(TAGS) -o $(BINARY)-linux-armv6 ./cli
	GOOS=linux   GOARM=7 GOARCH=arm $(GO_BUILD) $(TAGS) -o $(BINARY)-linux-armv7 ./cli
	GOOS=darwin  GOARCH=amd64 $(GO_BUILD) $(TAGS) -o $(BINARY)-darwin-amd64 ./cli
	GOOS=darwin  GOARCH=arm64 $(GO_BUILD) $(TAGS) -o $(BINARY)-darwin-arm64 ./cli
	GOOS=windows GOARCH=amd64 $(GO_BUILD) $(TAGS) -o $(BINARY)-windows-amd64.exe ./cli

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

.PHONY: package
package: cross
	mkdir -p dist
	tar -czf dist/docker-linux-amd64.tar.gz $(TAR_TRANSFORM) packaging/LICENSE $(BINARY)-linux-amd64
	tar -czf dist/docker-linux-arm64.tar.gz $(TAR_TRANSFORM) packaging/LICENSE $(BINARY)-linux-arm64
	tar -czf dist/docker-linux-armv6.tar.gz $(TAR_TRANSFORM) packaging/LICENSE $(BINARY)-linux-armv6
	tar -czf dist/docker-linux-armv7.tar.gz $(TAR_TRANSFORM) packaging/LICENSE $(BINARY)-linux-armv7
	tar -czf dist/docker-darwin-amd64.tar.gz $(TAR_TRANSFORM) packaging/LICENSE $(BINARY)-darwin-amd64
	tar -czf dist/docker-darwin-arm64.tar.gz $(TAR_TRANSFORM) packaging/LICENSE $(BINARY)-darwin-arm64
	cp $(BINARY)-windows-amd64.exe $(WORK_DIR)/docker.exe
	rm -f dist/docker-windows-amd64.zip && zip dist/docker-windows-amd64.zip -j packaging/LICENSE $(WORK_DIR)/docker.exe
	rm -r $(WORK_DIR)

.PHONY: yamldocs
yamldocs:
	go run docs/yaml/main/generate.go