// Copyright 2022 Docker Compose CLI authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

variable "GO_VERSION" {
  # default ARG value set in Dockerfile
  default = null
}

variable "BUILD_TAGS" {
  default = "e2e"
}

variable "DOCS_FORMATS" {
  default = "md,yaml"
}

# Defines the output folder to override the default behavior.
# See Makefile for details, this is generally only useful for
# the packaging scripts and care should be taken to not break
# them.
variable "DESTDIR" {
  default = ""
}
function "outdir" {
  params = [defaultdir]
  result = DESTDIR != "" ? DESTDIR : "${defaultdir}"
}

# Special target: https://github.com/docker/metadata-action#bake-definition
target "meta-helper" {}

target "_common" {
  args = {
    GO_VERSION = GO_VERSION
    BUILD_TAGS = BUILD_TAGS
    BUILDKIT_CONTEXT_KEEP_GIT_DIR = 1
  }
}

group "default" {
  targets = ["binary"]
}

group "validate" {
  targets = ["lint", "vendor-validate", "license-validate", "mocks-validate", "relay-lint"]
}

target "lint" {
  inherits = ["_common"]
  target = "lint"
  output = ["type=cacheonly"]
}

target "license-validate" {
  target = "license-validate"
  output = ["type=cacheonly"]
}

target "license-update" {
  target = "license-update"
  output = ["."]
}

target "vendor-validate" {
  inherits = ["_common"]
  target = "vendor-validate"
  output = ["type=cacheonly"]
}

target "mocks-validate" {
  inherits = ["_common"]
  target = "mocks-validate"
  output = ["type=cacheonly"]
}

target "vendor-update" {
  inherits = ["_common"]
  target = "vendor-update"
  output = ["."]
}

target "test" {
  inherits = ["_common"]
  target = "test-coverage"
  output = [outdir("./bin/coverage/unit")]
}

target "binary-with-coverage" {
  inherits = ["_common"]
  target = "binary"
  args = {
    BUILD_FLAGS = "-cover -covermode=atomic"
  }
  output = [outdir("./bin/build")]
  platforms = ["local"]
}

target "binary" {
  inherits = ["_common"]
  target = "binary"
  output = [outdir("./bin/build")]
  platforms = ["local"]
}

target "binary-cross" {
  inherits = ["binary"]
  platforms = [
    "darwin/amd64",
    "darwin/arm64",
    "linux/amd64",
    "linux/arm/v6",
    "linux/arm/v7",
    "linux/arm64",
    "linux/ppc64le",
    "linux/riscv64",
    "linux/s390x",
    "windows/amd64",
    "windows/arm64"
  ]
}

target "release" {
  inherits = ["binary-cross"]
  target = "release"
  output = [outdir("./bin/release")]
}

target "docs-validate" {
  inherits = ["_common"]
  target = "docs-validate"
  output = ["type=cacheonly"]
}

target "docs-update" {
  inherits = ["_common"]
  target = "docs-update"
  output = ["./docs"]
}

target "image-cross" {
  inherits = ["meta-helper", "binary-cross"]
  output = ["type=image"]
}

target "image-module-cross" {
  inherits = ["meta-helper", "binary-cross"]
  target = "module"
  output = ["type=image"]
  platforms = [
    "darwin/amd64",
    "darwin/arm64",
    "linux/amd64",
    "linux/arm64",
    "windows/amd64",
    "windows/arm64",
  ]
}

// relay-lint and relay-test validate relay/'s own module (a separate go.mod:
// go vet/lint/test from the repo root don't cover it). Self-contained build
// context, so relay/ carries its own .golangci.yml rather than sharing the
// root one, which isn't in scope for a "./relay" context.
target "relay-lint" {
  context = "./relay"
  target  = "lint"
  output  = ["type=cacheonly"]
}

target "relay-test" {
  context = "./relay"
  target  = "test"
  output  = ["type=cacheonly"]
}

// relay-image is the local/dev build of the network relay compose deploys in
// place of a provider service that published endpoints (see relay/). The tag
// matches the runtime default (COMPOSE_RELAY_IMAGE overrides it).
target "relay-image" {
  context = "./relay"
  tags = ["docker/compose-relay:v1"]
}

// relay-image-cross is the CI publication target: tags and labels come from
// the workflow through meta-helper, platforms cover every linux platform the
// compose binary ships for — the relay runs as a container on the engine, so
// darwin/windows binaries make no sense for it.
target "relay-image-cross" {
  inherits = ["meta-helper"]
  context = "./relay"
  output = ["type=image"]
  platforms = [
    "linux/amd64",
    "linux/arm/v6",
    "linux/arm/v7",
    "linux/arm64",
    "linux/ppc64le",
    "linux/riscv64",
    "linux/s390x",
  ]
}
