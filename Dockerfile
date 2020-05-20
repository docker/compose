# syntax = docker/dockerfile:experimental
ARG GO_VERSION=1.14.3-alpine3.11

FROM golang:${GO_VERSION} AS base
ARG TARGET_OS=unknown
ARG TARGET_ARCH=unknown
ARG PWD=/api
ENV GO111MODULE=on

RUN apk update && apk add docker make

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

RUN go get github.com/golang/protobuf/protoc-gen-go@v1.4.1 && \
    go get golang.org/x/tools/cmd/goimports

WORKDIR ${PWD}
ADD go.* ${PWD}
ADD . ${PWD}

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
ENV CGO_ENABLED=0
RUN make -f builder.Makefile test
