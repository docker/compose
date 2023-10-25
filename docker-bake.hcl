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
  targets = ["lint", "vendor-validate", "license-validate"]
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
