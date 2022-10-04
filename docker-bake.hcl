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
  default = "1.19.2"
}

variable "BUILD_TAGS" {
  default = "e2e,kube"
}

variable "DOCS_FORMATS" {
  default = "md,yaml"
}

# Defines the output folder
variable "DESTDIR" {
  default = ""
}
function "bindir" {
  params = [defaultdir]
  result = DESTDIR != "" ? DESTDIR : "./bin/${defaultdir}"
}

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

target "modules-validate" {
  inherits = ["_common"]
  target = "modules-validate"
  output = ["type=cacheonly"]
}

target "vendor" {
  inherits = ["_common"]
  target = "vendor-update"
  output = ["."]
}

target "test" {
  inherits = ["_common"]
  target = "test-coverage"
  output = [bindir("coverage")]
}

target "binary" {
  inherits = ["_common"]
  target = "binary"
  output = [bindir("build")]
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
  output = [bindir("release")]
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
