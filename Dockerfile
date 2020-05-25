# syntax = docker/dockerfile:experimental
ARG GO_VERSION=1.14.3-alpine
ARG GOLANGCI_LINT_VERSION=1.27.0

FROM golang:${GO_VERSION} AS base
ARG TARGET_OS=unknown
ARG TARGET_ARCH=unknown
ARG PWD=/api
ENV GO111MODULE=on

RUN apk update && apk add -U docker make

WORKDIR ${PWD}
ADD go.* ${PWD}
RUN go mod download
ADD . ${PWD}

FROM golang:${GO_VERSION} AS protos-base
ARG TARGET_OS=unknown
ARG TARGET_ARCH=unknown
ARG PWD=/api
ENV GO111MODULE=on

RUN apk update && apk add protoc make

RUN go get github.com/golang/protobuf/protoc-gen-go@v1.4.1

WORKDIR ${PWD}
ADD go.* ${PWD}
ADD . ${PWD}

FROM protos-base AS make-protos
RUN make -f builder.Makefile protos

FROM golangci/golangci-lint:v${GOLANGCI_LINT_VERSION}-alpine AS lint-base

FROM base AS lint
COPY --from=lint-base /usr/bin/golangci-lint /usr/bin/golangci-lint
ENV CGO_ENABLED=0
RUN --mount=id=build,type=cache,target=/root/.cache/go-build \
    --mount=id=lint,type=cache,target=/root/.cache/golangci-lint \
    make -f builder.Makefile lint

FROM base AS make-cli
RUN --mount=id=build,type=cache,target=/root/.cache/go-build \
    GOOS=${TARGET_OS} \
    GOARCH=${TARGET_ARCH} \
    make -f builder.Makefile cli

FROM base AS make-cross
RUN --mount=id=build,type=cache,target=/root/.cache/go-build \
    make -f builder.Makefile cross

FROM scratch AS protos
COPY --from=make-protos /api/protos .

FROM scratch AS cli
COPY --from=make-cli /api/bin/* .

FROM scratch AS cross
COPY --from=make-cross /api/bin/* .

FROM base as test
ENV CGO_ENABLED=0
RUN --mount=id=build,type=cache,target=/root/.cache/go-build \
    make -f builder.Makefile test
