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

ARG GO_VERSION=1.18.5
ARG FORMATS=md,yaml

FROM --platform=${BUILDPLATFORM} golang:${GO_VERSION}-alpine AS docsgen
WORKDIR /src
RUN --mount=target=. \
  --mount=target=/root/.cache,type=cache \
  go build -o /out/docsgen ./docs/yaml/main/generate.go

FROM --platform=${BUILDPLATFORM} alpine AS gen
RUN apk add --no-cache rsync git
WORKDIR /src
COPY --from=docsgen /out/docsgen /usr/bin
ARG FORMATS
RUN --mount=target=/context \
  --mount=target=.,type=tmpfs <<EOT
set -e
rsync -a /context/. .
docsgen --formats "$FORMATS" --source "docs/reference"
mkdir /out
cp -r docs/reference /out
EOT

FROM scratch AS update
COPY --from=gen /out /out

FROM gen AS validate
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
