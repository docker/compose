# syntax = docker/dockerfile:experimental
ARG GO_VERSION=1.14.4-alpine
ARG ALPINE_PKG_DOCKER_VERSION=19.03.11-r0
ARG GOLANGCI_LINT_VERSION=v1.27.0-alpine

FROM --platform=${BUILDPLATFORM} golang:${GO_VERSION} AS base
WORKDIR /ecs-plugin
ENV GO111MODULE=on
ARG ALPINE_PKG_DOCKER_VERSION
RUN apk add --no-cache \
    docker=${ALPINE_PKG_DOCKER_VERSION} \
    make
COPY go.* .
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .

FROM base AS make-plugin
ARG TARGETOS
ARG TARGETARCH
RUN apk add build-base
RUN GO111MODULE=on go get github.com/golang/mock/mockgen@latest
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    GOOS=${TARGETOS} \
    GOARCH=${TARGETARCH} \
    make -f builder.Makefile build

FROM base AS make-cross
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    make -f builder.Makefile cross

FROM scratch AS build
COPY --from=make-plugin /ecs-plugin/dist/docker-ecs .

FROM scratch AS cross
COPY --from=make-cross /ecs-plugin/dist/* .

FROM make-plugin AS test
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    make -f builder.Makefile test

FROM golangci/golangci-lint:${GOLANGCI_LINT_VERSION} AS lint-base

FROM base AS lint
COPY --from=lint-base /usr/bin/golangci-lint /usr/bin/golangci-lint
RUN --mount=target=. \
    --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/golangci-lint \
    make -f builder.Makefile lint
