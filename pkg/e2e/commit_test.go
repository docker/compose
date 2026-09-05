//go:build e2e

/*
   Copyright 2023 Docker Compose CLI authors

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
	"testing"
)

func TestCommit(t *testing.T) {
	s := NewScenario(t, "commit must produce an image from the service's container")
	image := s.Project() + ":latest"
	s.Defer(DockerCmd("image", "rm", "-f", image).MayFail()).
		Step("up starts the service",
			ComposeCmd("up", "-d", "service"),
			ServiceState("service", "running")).
		Step("commit turns the container into a tagged image",
			ComposeCmd("commit",
				"-a", "John Hannibal Smith <hannibal@a-team.com>",
				"-c", "ENV DEBUG=true",
				"-m", "sample commit",
				"service", image),
			ImageExists(image))
}

func TestCommitWithReplicas(t *testing.T) {
	s := NewScenario(t, "commit --index must pick the requested replica of a scaled service")
	image1, image2 := s.Project()+":1", s.Project()+":2"
	s.Defer(
		DockerCmd("image", "rm", "-f", image1).MayFail(),
		DockerCmd("image", "rm", "-f", image2).MayFail()).
		Step("up starts the replicas",
			ComposeCmd("up", "-d", "service-with-replicas"),
			ServiceScale("service-with-replicas", 3)).
		Step("commit --index=1 images the first replica",
			ComposeCmd("commit", "-m", "sample commit", "--index=1", "service-with-replicas", image1),
			ImageExists(image1)).
		Step("commit --index=2 images the second replica",
			ComposeCmd("commit", "-m", "sample commit", "--index=2", "service-with-replicas", image2),
			ImageExists(image2))
}
