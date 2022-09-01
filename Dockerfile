ARG GO_VERSION=1.17-alpine
ARG GOLANGCI_LINT_VERSION=v1.40.1-alpine
ARG PROTOC_GEN_GO_VERSION=v1.4.3

FROM --platform=${BUILDPLATFORM} golang:${GO_VERSION} AS base
WORKDIR /compose-cli
RUN apk add --no-cache -vv \
    git \
    docker \
    make \
    protoc \
    protobuf-dev
COPY go.* .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

FROM base AS make-compose-plugin
ENV CGO_ENABLED=0
ARG TARGETOS
ARG TARGETARCH
ARG BUILD_TAGS
ARG GIT_TAG
RUN --mount=target=. \
    --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=${TARGETOS} \
    GOARCH=${TARGETARCH} \
    BUILD_TAGS=${BUILD_TAGS} \
    GIT_TAG=${GIT_TAG} \
    make COMPOSE_BINARY=/out/docker-compose -f builder.Makefile compose-plugin

FROM alpine:3.16 AS compose-plugin
WORKDIR /root
COPY --from=make-compose-plugin /out/* /usr/local/bin/

RUN adduser -D -h /home/cfu -s /bin/bash cfu
USER cfu

ENTRYPOINT [ "docker-compose" ]
