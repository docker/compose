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

# Used for docker-ce-packaging where builds cannot use Docker
#
# See:
# 	- https://github.com/docker/docker-ce-packaging/blob/20.10/rpm/SPECS/docker-compose-plugin.spec
# 	- https://github.com/docker/docker-ce-packaging/blob/20.10/deb/common/rules

GOOS?=$(shell go env GOOS)
GOARCH?=$(shell go env GOARCH)

EXTENSION:=
ifeq ($(GOOS),windows)
  EXTENSION:=.exe
endif

PKG_NAME:=github.com/docker/compose/v2
GIT_TAG?=$(shell git describe --tags --match "v[0-9]*")
LDFLAGS="-s -w -X $(PKG_NAME)/internal.Version=${GIT_TAG}"
STATIC_FLAGS=CGO_ENABLED=0
GO_BUILD=$(STATIC_FLAGS) go build -trimpath -ldflags=$(LDFLAGS)

COMPOSE_BINARY?=bin/build/docker-compose
COMPOSE_BINARY_WITH_EXTENSION=$(COMPOSE_BINARY)$(EXTENSION)

TAGS:=
ifdef BUILD_TAGS
  TAGS=-tags $(BUILD_TAGS)
endif

.PHONY: compose-plugin
compose-plugin:
	GOOS=${GOOS} GOARCH=${GOARCH} $(GO_BUILD) $(TAGS) -o $(COMPOSE_BINARY_WITH_EXTENSION) ./cmd
