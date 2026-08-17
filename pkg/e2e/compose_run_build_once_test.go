/*
   Copyright 2020 Docker Compose CLI authors

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package e2e

import (
	"regexp"
	"testing"
)

// The build-once scenarios lock a regression where run built pull_policy:
// build dependencies twice — once in startDependencies and once in
// ensureImagesExists. Counting the "naming to ... done" build reports in the
// verbose output is the observable.

func builtOnce(project, service string) Check {
	return OutputMatchesCount(`naming to .*`+regexp.QuoteMeta(project)+`-`+regexp.QuoteMeta(service)+`.* done`, 1)
}

func TestRunBuildOnceDependency(t *testing.T) {
	s := NewScenario(t, "run --build must build a pull_policy: build dependency exactly once")
	s.Compose(`
services:
  nginx:
    pull_policy: build
    build:
      dockerfile_inline: |
        FROM alpine
        RUN echo "Nginx built at $(date)" > /build-time.txt
        CMD sleep 3600

  curl:
    image: alpine
    depends_on:
      - nginx
    command: echo "curl service"
`).
		Defer(DockerCmd("image", "rm", "-f", s.Project()+"-nginx").MayFail()).
		Step("run builds the dependency once and executes the service",
			ComposeCmd("--verbose", "run", "--build", "--rm", "curl"),
			builtOnce(s.Project(), "nginx"),
			StdoutContains("curl service"))
}

func TestRunBuildOnceNestedDependencies(t *testing.T) {
	s := NewScenario(t, "run --build must build each service of a dependency chain exactly once")
	s.Compose(`
services:
  db:
    pull_policy: build
    build:
      dockerfile_inline: |
        FROM alpine
        RUN echo "DB built at $(date)" > /db-build.txt
        CMD sleep 3600

  api:
    pull_policy: build
    build:
      dockerfile_inline: |
        FROM alpine
        RUN echo "API built at $(date)" > /api-build.txt
        CMD sleep 3600
    depends_on:
      - db

  app:
    pull_policy: build
    build:
      dockerfile_inline: |
        FROM alpine
        RUN echo "App built at $(date)" > /app-build.txt
        CMD echo "App running"
    depends_on:
      - api
`).
		Defer(
			DockerCmd("image", "rm", "-f", s.Project()+"-db").MayFail(),
			DockerCmd("image", "rm", "-f", s.Project()+"-api").MayFail(),
			DockerCmd("image", "rm", "-f", s.Project()+"-app").MayFail()).
		Step("run builds the whole chain, each image once",
			ComposeCmd("--verbose", "run", "--build", "--rm", "app"),
			builtOnce(s.Project(), "db"),
			builtOnce(s.Project(), "api"),
			builtOnce(s.Project(), "app"),
			StdoutContains("App running"))
}

func TestRunBuildOnceNoDeps(t *testing.T) {
	s := NewScenario(t, "run --build on a dependency-less service must build it exactly once")
	s.Compose(`
services:
  simple:
    pull_policy: build
    build:
      dockerfile_inline: |
        FROM alpine
        RUN echo "Simple built at $(date)" > /build.txt
        CMD echo "Simple service"
`).
		Defer(DockerCmd("image", "rm", "-f", s.Project()+"-simple").MayFail()).
		Step("run builds the service once and executes it",
			ComposeCmd("run", "--build", "--rm", "simple"),
			builtOnce(s.Project(), "simple"),
			StdoutContains("Simple service"))
}
