# syntax=docker/dockerfile:1


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

ARG GO_VERSION=1.19.4-alpine
ARG XX_VERSION=1.1.2
ARG GOLANGCI_LINT_VERSION=v1.49.0
ARG ADDLICENSE_VERSION=v1.0.0

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

FROM debian:bullseye-slim AS compose-plugin

COPY --from=make-compose-plugin /out/* /usr/local/bin/
# add user
RUN addgroup --gid 3000 compose && \
  adduser --uid 3000 --gecos "" --disabled-password \
  --ingroup compose \
  --home /home/compose \
  --shell /bin/bash compose

WORKDIR /home/compose

RUN chown -R compose:compose /home/compose && \
  chmod 755 /home/compose


USER compose:compose

ENTRYPOINT [ "docker-compose" ]
