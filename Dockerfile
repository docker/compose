# syntax=docker/dockerfile:experimental


#   Copyright 2020 Docker, Inc.

#   Licensed under the Apache License, Version 2.0 (the "License");
#   you may not use this file except in compliance with the License.
#   You may obtain a copy of the License at

#       http://www.apache.org/licenses/LICENSE-2.0

#   Unless required by applicable law or agreed to in writing, software
#   distributed under the License is distributed on an "AS IS" BASIS,
#   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#   See the License for the specific language governing permissions and
#   limitations under the License.
ARG GO_VERSION=1.15.2-alpine
ARG GOLANGCI_LINT_VERSION=v1.31.0-alpine

FROM --platform=${BUILDPLATFORM} golang:${GO_VERSION} AS base
WORKDIR /compose-cli
ENV GO111MODULE=on
RUN apk add --no-cache \
    git \
    docker \
    make \
    protoc \
    protobuf-dev
COPY go.* .
RUN go mod download

FROM base AS make-protos
RUN go get github.com/golang/protobuf/protoc-gen-go@v1.4.1
COPY . .
RUN make -f builder.Makefile protos

FROM golangci/golangci-lint:${GOLANGCI_LINT_VERSION} AS lint-base

FROM base AS lint
ENV CGO_ENABLED=0
COPY --from=lint-base /usr/bin/golangci-lint /usr/bin/golangci-lint
ARG GIT_TAG
RUN --mount=target=. \
    --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/root/.cache/golangci-lint \
    GIT_TAG=${GIT_TAG} \
    make -f builder.Makefile lint

FROM base AS import-restrictions-base
RUN go get github.com/docker/import-restrictions

FROM import-restrictions-base AS import-restrictions
RUN --mount=target=. \
    make -f builder.Makefile import-restrictions

FROM base AS make-cli
ENV CGO_ENABLED=0
ARG TARGETOS
ARG TARGETARCH
ARG BUILD_TAGS
ARG GIT_TAG
RUN --mount=target=. \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=${TARGETOS} \
    GOARCH=${TARGETARCH} \
    BUILD_TAGS=${BUILD_TAGS} \
    GIT_TAG=${GIT_TAG} \
    make BINARY=/out/docker -f builder.Makefile cli

FROM base AS make-cross
ARG BUILD_TAGS
ARG GIT_TAG
RUN --mount=target=. \
    --mount=type=cache,target=/root/.cache/go-build \
    BUILD_TAGS=${BUILD_TAGS} \
    GIT_TAG=${GIT_TAG} \
    make BINARY=/out/docker  -f builder.Makefile cross

FROM scratch AS protos
COPY --from=make-protos /compose-cli/protos .

FROM scratch AS cli
COPY --from=make-cli /out/* .

FROM scratch AS cross
COPY --from=make-cross /out/* .

FROM base as test
ENV CGO_ENABLED=0
ARG BUILD_TAGS
ARG GIT_TAG
RUN --mount=target=. \
    --mount=type=cache,target=/root/.cache/go-build \
    BUILD_TAGS=${BUILD_TAGS} \
    GIT_TAG=${GIT_TAG} \
    make -f builder.Makefile test

FROM base as check-license-headers
RUN go get -u github.com/kunalkushwaha/ltag
RUN --mount=target=. \
    --mount=type=cache,target=/root/.cache/go-build \
    make -f builder.Makefile check-license-headers

FROM base as check-go-mod
COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    make -f builder.Makefile check-go-mod
