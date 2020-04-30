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

GIT_COMMIT=$(shell git rev-parse --short HEAD)
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

PROTOS=$(shell find . -name \*.proto)

export DOCKER_BUILDKIT=1

all: dbins
xall: dxbins
bins: cli example
xbins: xcli xexample

protos:
	@protoc -I. --go_out=plugins=grpc,paths=source_relative:. ${PROTOS}

cli: protos
	GOOS=${GOOS} GOARCH=${GOARCH} go build -v -o bin/docker ./cli

xcli: cli
	GOOS=linux   GOARCH=amd64 go build -v -o bin/docker-linux-amd64 ./cli
	GOOS=darwin  GOARCH=amd64 go build -v -o bin/docker-darwin-amd64 ./cli
	GOOS=windows GOARCH=amd64 go build -v -o bin/docker-windows-amd64.exe ./cli

dprotos:
	docker build . \
	--output type=local,dest=. \
	--target protos

dbins: dprotos
	docker build . \
	--output type=local,dest=./bin \
	--build-arg TARGET_OS=${GOOS} \
	--build-arg TARGET_ARCH=${GOARCH} \
	--target bins

dxbins: dbins
	docker build . \
	--output type=local,dest=./bin \
	--target xbins

dtest:
	docker build . \
	--target make-test

test:
	gotestsum ./...

FORCE:

.PHONY: all xall protos xcli cli bins dbins dxbins dprotos
