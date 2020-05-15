# syntax = docker/dockerfile:experimental
ARG GO_VERSION=1.14.2

FROM golang:${GO_VERSION} AS base
ARG TARGET_OS=unknown
ARG TARGET_ARCH=unknown
ARG PWD=/api
ENV GO111MODULE=on

WORKDIR ${PWD}
ADD go.* ${PWD}
ADD . ${PWD}

FROM golang:${GO_VERSION} AS protos-base
ARG TARGET_OS=unknown
ARG TARGET_ARCH=unknown
ARG PWD=/api
ENV GO111MODULE=on

RUN apt-get update && apt-get install --no-install-recommends -y \
    protobuf-compiler \
    libprotobuf-dev

RUN go get github.com/golang/protobuf/protoc-gen-go@v1.4.1 && \
    go get golang.org/x/tools/cmd/goimports

WORKDIR ${PWD}
ADD go.* ${PWD}
ADD . ${PWD}

FROM golang:${GO_VERSION} AS lint-base
RUN go get github.com/golangci/golangci-lint/cmd/golangci-lint@v1.26.0

FROM protos-base AS make-protos
RUN make -f builder.Makefile protos

FROM base AS make-cli
RUN --mount=type=cache,target=/root/.cache/go-build \
    GOOS=${TARGET_OS} \
    GOARCH=${TARGET_ARCH} \
    make -f  builder.Makefile cli

FROM base AS make-cross
RUN --mount=type=cache,target=/root/.cache/go-build \
    make -f builder.Makefile cross

FROM scratch AS protos
COPY --from=make-protos /api .

FROM scratch AS cli
COPY --from=make-cli /api/bin/* .

FROM scratch AS cross
COPY --from=make-cross /api/bin/* .

FROM base as test
RUN make -f builder.Makefile test

FROM lint-base AS lint
RUN make -f builder.Makefile lint
