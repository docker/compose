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

# Keep the versions below in sync with the Dockerfile at the root: the relay is
# built the way the compose binary is, on the build platform, cross-compiled
# for the target with xx. A golang image per target platform does not scale: the
# alpine tags do not carry every platform the relay ships for (riscv64 is
# missing from golang:1.27-alpine).
ARG GO_VERSION=1.27.4
ARG XX_VERSION=1.9.0
ARG GOLANGCI_LINT_VERSION=v2.15.0

# xx is a helper for cross-compilation
FROM --platform=${BUILDPLATFORM} tonistiigi/xx:${XX_VERSION} AS xx

FROM golangci/golangci-lint:${GOLANGCI_LINT_VERSION}-alpine AS golangci-lint

FROM --platform=${BUILDPLATFORM} golang:${GO_VERSION}-alpine3.23 AS base
COPY --from=xx / /
WORKDIR /src
ENV CGO_ENABLED=0

FROM base AS build-base
COPY go.mod ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

FROM build-base AS lint
ENV GOLANGCI_LINT_CACHE=/cache/golangci-lint
RUN --mount=type=bind,target=. \
    --mount=type=cache,target=/root/.cache \
    --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/cache/golangci-lint \
    --mount=from=golangci-lint,source=/usr/bin/golangci-lint,target=/usr/bin/golangci-lint \
    golangci-lint run ./...

FROM build-base AS test
RUN --mount=type=bind,target=. \
    --mount=type=cache,target=/root/.cache \
    --mount=type=cache,target=/go/pkg/mod \
    go vet ./... && \
    go test -v ./...

FROM base AS build
ARG TARGETPLATFORM
COPY go.mod main.go ./
RUN xx-go build -trimpath -ldflags="-w -s" -o /compose-relay . && \
    xx-verify --static /compose-relay

FROM scratch
COPY --from=build /compose-relay /compose-relay
USER 65532:65532
ENTRYPOINT ["/compose-relay"]
