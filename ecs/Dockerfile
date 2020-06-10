# syntax = docker/dockerfile:experimental
ARG GO_VERSION=1.14.2

FROM golang:${GO_VERSION} AS base
ARG TARGET_OS=unknown
ARG TARGET_ARCH=unknown
ARG PWD=/ecs-plugin
ENV GO111MODULE=on

WORKDIR ${PWD}
ADD go.* ${PWD}
RUN go mod download
ADD . ${PWD}

FROM base AS make-plugin
RUN --mount=type=cache,target=/root/.cache/go-build \
    GOOS=${TARGET_OS} \
    GOARCH=${TARGET_ARCH} \
    make -f builder.Makefile build

FROM base AS make-cross
RUN --mount=type=cache,target=/root/.cache/go-build \
    make -f builder.Makefile cross

FROM scratch AS build
COPY --from=make-plugin /ecs-plugin/dist/* .

FROM scratch AS cross
COPY --from=make-cross /ecs-plugin/dist/* .

FROM base as test
RUN make -f builder.Makefile test
