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

ARG GO_VERSION=1.19.2
ARG XX_VERSION=1.1.2
ARG GOLANGCI_LINT_VERSION=v1.49.0
ARG ADDLICENSE_VERSION=v1.0.0

ARG BUILD_TAGS="e2e,kube"
ARG DOCS_FORMATS="md,yaml"
ARG LICENSE_FILES=".*\(Dockerfile\|Makefile\|\.go\|\.hcl\|\.sh\)"

# xx is a helper for cross-compilation
FROM --platform=${BUILDPLATFORM} tonistiigi/xx:${XX_VERSION} AS xx

FROM golangci/golangci-lint:${GOLANGCI_LINT_VERSION}-alpine AS golangci-lint
FROM ghcr.io/google/addlicense:${ADDLICENSE_VERSION} AS addlicense

FROM --platform=${BUILDPLATFORM} golang:${GO_VERSION}-alpine AS base
COPY --from=xx / /
RUN apk add --no-cache \
      docker \
      file \
      git \
      make \
      protoc \
      protobuf-dev
WORKDIR /src
ENV CGO_ENABLED=0

FROM base AS build-base
COPY go.* .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

FROM build-base AS vendored
RUN --mount=type=bind,target=.,rw \
    --mount=type=cache,target=/go/pkg/mod \
    go mod tidy && mkdir /out && cp go.mod go.sum /out

FROM scratch AS vendor-update
COPY --from=vendored /out /

FROM vendored AS vendor-validate
RUN --mount=type=bind,target=.,rw <<EOT
  set -e
  git add -A
  cp -rf /out/* .
  diff=$(git status --porcelain -- go.mod go.sum)
  if [ -n "$diff" ]; then
    echo >&2 'ERROR: Vendor result differs. Please vendor your package with "make go-mod-tidy"'
    echo "$diff"
    exit 1
  fi
EOT

FROM vendored AS modules-validate
RUN apk add --no-cache bash
RUN apk add --no-cache jq
RUN --mount=type=bind,target=.,rw ./verify-go-modules.sh e2e

FROM build-base AS build
ARG BUILD_TAGS
ARG TARGETPLATFORM
RUN xx-go --wrap
RUN --mount=type=bind,target=. \
    --mount=type=cache,target=/root/.cache \
    --mount=type=cache,target=/go/pkg/mod \
    make build GO_BUILDTAGS="$BUILD_TAGS" DESTDIR=/usr/bin && \
    xx-verify --static /usr/bin/docker-compose

FROM build-base AS lint
ARG BUILD_TAGS
RUN --mount=type=bind,target=. \
    --mount=type=cache,target=/root/.cache \
    --mount=from=golangci-lint,source=/usr/bin/golangci-lint,target=/usr/bin/golangci-lint \
    golangci-lint run --build-tags "$BUILD_TAGS" ./...

FROM build-base AS test
ARG CGO_ENABLED=0
ARG BUILD_TAGS
RUN --mount=type=bind,target=. \
    --mount=type=cache,target=/root/.cache \
    --mount=type=cache,target=/go/pkg/mod \
    go test -tags "$BUILD_TAGS" -v -coverprofile=/tmp/coverage.txt -covermode=atomic $(go list  $(TAGS) ./... | grep -vE 'e2e') && \
    go tool cover -func=/tmp/coverage.txt

FROM scratch AS test-coverage
COPY --from=test /tmp/coverage.txt /coverage.txt

FROM base AS license-set
ARG LICENSE_FILES
RUN --mount=type=bind,target=.,rw \
    --mount=from=addlicense,source=/app/addlicense,target=/usr/bin/addlicense \
    find . -regex "${LICENSE_FILES}" | xargs addlicense -c 'Docker Compose CLI' -l apache && \
    mkdir /out && \
    find . -regex "${LICENSE_FILES}" | cpio -pdm /out

FROM scratch AS license-update
COPY --from=set /out /

FROM base AS license-validate
ARG LICENSE_FILES
RUN --mount=type=bind,target=. \
    --mount=from=addlicense,source=/app/addlicense,target=/usr/bin/addlicense \
    find . -regex "${LICENSE_FILES}" | xargs addlicense -check -c 'Docker Compose CLI' -l apache -ignore validate -ignore testdata -ignore resolvepath -v

FROM base AS docsgen
WORKDIR /src
RUN --mount=target=. \
    --mount=target=/root/.cache,type=cache \
    go build -o /out/docsgen ./docs/yaml/main/generate.go

FROM --platform=${BUILDPLATFORM} alpine AS docs-build
RUN apk add --no-cache rsync git
WORKDIR /src
COPY --from=docsgen /out/docsgen /usr/bin
ARG DOCS_FORMATS
RUN --mount=target=/context \
    --mount=target=.,type=tmpfs <<EOT
  set -e
  rsync -a /context/. .
  docsgen --formats "$DOCS_FORMATS" --source "docs/reference"
  mkdir /out
  cp -r docs/reference /out
EOT

FROM scratch AS docs-update
COPY --from=docs-build /out /out

FROM docs-build AS docs-validate
RUN --mount=target=/context \
    --mount=target=.,type=tmpfs <<EOT
  set -e
  rsync -a /context/. .
  git add -A
  rm -rf docs/reference/*
  cp -rf /out/* ./docs/
  if [ -n "$(git status --porcelain -- docs/reference)" ]; then
    echo >&2 'ERROR: Docs result differs. Please update with "make docs"'
    git status --porcelain -- docs/reference
    exit 1
  fi
EOT

FROM scratch AS binary-unix
COPY --link --from=build /usr/bin/docker-compose /
FROM binary-unix AS binary-darwin
FROM binary-unix AS binary-linux
FROM scratch AS binary-windows
COPY --link --from=build /usr/bin/docker-compose /docker-compose.exe
FROM binary-$TARGETOS AS binary

FROM --platform=$BUILDPLATFORM alpine AS releaser
WORKDIR /work
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
RUN --mount=from=binary \
    mkdir -p /out && \
    # TODO: should just use standard arch
    TARGETARCH=$([ "$TARGETARCH" = "amd64" ] && echo "x86_64" || echo "$TARGETARCH"); \
    TARGETARCH=$([ "$TARGETARCH" = "arm64" ] && echo "aarch64" || echo "$TARGETARCH"); \
    cp docker-compose* "/out/docker-compose-${TARGETOS}-${TARGETARCH}${TARGETVARIANT}$(ls docker-compose* | sed -e 's/^docker-compose//')"

FROM scratch AS release
COPY --from=releaser /out/ /

# docs-reference is a target used as remote context to update docs on release
# with latest changes on docs.docker.com.
# see open-pr job in .github/workflows/docs.yml for more details
FROM scratch AS docs-reference
COPY docs/reference/*.yaml .
