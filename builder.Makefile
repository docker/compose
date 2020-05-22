# Copyright (c) 2020 Docker Inc.

# Permission is hereby granted, free of charge, to any person
# obtaining a copy of this software and associated documentation
# files (the "Software"), to deal in the Software without
# restriction, including without limitation the rights to use, copy,
# modify, merge, publish, distribute, sublicense, and/or sell copies
# of the Software, and to permit persons to whom the Software is
# furnished to do so, subject to the following conditions:

# The above copyright notice and this permission notice shall be
# included in all copies or substantial portions of the Software.

# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
# EXPRESS OR IMPLIED,
# INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
# FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
# IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT
# HOLDERS BE LIABLE FOR ANY CLAIM,
# DAMAGES OR OTHER LIABILITY,
# WHETHER IN AN ACTION OF CONTRACT,
# TORT OR OTHERWISE,
# ARISING FROM, OUT OF OR IN CONNECTION WITH
# THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

PROTOS=$(shell find . -not \( -path ./tests -prune \) -name \*.proto)

EXTENSION :=
ifeq ($(GOOS),windows)
  EXTENSION := .exe
endif

STATIC_FLAGS=CGO_ENABLED=0
LDFLAGS := "-s -w"
GO_BUILD = $(STATIC_FLAGS) go build -trimpath -ldflags=$(LDFLAGS)

BINARY=bin/docker
BINARY_WITH_EXTENSION=$(BINARY)$(EXTENSION)

all: cli

protos:
	@protoc -I. --go_out=plugins=grpc,paths=source_relative:. ${PROTOS}
	@goimports -w -local github.com/docker/api .

cli:
	GOOS=${GOOS} GOARCH=${GOARCH} $(GO_BUILD) -o $(BINARY_WITH_EXTENSION) ./cli

cross:
	@GOOS=linux   GOARCH=amd64 $(GO_BUILD) -o $(BINARY)-linux-amd64 ./cli
	@GOOS=darwin  GOARCH=amd64 $(GO_BUILD) -o $(BINARY)-darwin-amd64 ./cli
	@GOOS=windows GOARCH=amd64 $(GO_BUILD) -o $(BINARY)-windows-amd64.exe ./cli

test:
	@go test -cover $(shell go list ./... | grep -vE 'e2e')

lint:
	golangci-lint run --timeout 10m0s ./...

FORCE:

.PHONY: all protos cli cross test lint
