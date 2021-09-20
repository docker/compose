# syntax=docker/dockerfile:1.2


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

ARG GO_VERSION=1.16-alpine
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

FROM base AS lint
ENV CGO_ENABLED=0
COPY --from=golangci/golangci-lint /usr/bin/golangci-lint /usr/bin/golangci-lint
ARG BUILD_TAGS
ARG GIT_TAG
RUN --mount=target=. \
    --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/root/.cache/golangci-lint \
    BUILD_TAGS=${BUILD_TAGS} \
    GIT_TAG=${GIT_TAG} \
    make -f builder.Makefile lint

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

FROM base AS make-cross
ARG BUILD_TAGS
ARG GIT_TAG
RUN --mount=target=. \
    --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    BUILD_TAGS=${BUILD_TAGS} \
    GIT_TAG=${GIT_TAG} \
    make COMPOSE_BINARY=/out/docker-compose -f builder.Makefile cross

FROM scratch AS compose-plugin
COPY --from=make-compose-plugin /out/* .

FROM scratch AS cross
COPY --from=make-cross /out/* .

FROM base AS test
ENV CGO_ENABLED=0
ARG BUILD_TAGS
ARG GIT_TAG
RUN --mount=target=. \
    --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    BUILD_TAGS=${BUILD_TAGS} \
    GIT_TAG=${GIT_TAG} \
    make -f builder.Makefile test

FROM base AS check-license-headers
RUN go get -u github.com/kunalkushwaha/ltag
RUN --mount=target=. \
    make -f builder.Makefile check-license-headers

FROM base AS make-go-mod-tidy
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod tidy

FROM scratch AS go-mod-tidy
COPY --from=make-go-mod-tidy /compose-cli/go.mod .
COPY --from=make-go-mod-tidy /compose-cli/go.sum .

FROM base AS check-go-mod
COPY . .
RUN make -f builder.Makefile check-go-mod
