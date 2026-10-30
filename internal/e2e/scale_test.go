//go:build e2e

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
	"testing"
)

func TestScaleBasicCases(t *testing.T) {
	NewScenario(t, "scale must add and remove replicas to match the requested count, per service").
		Step("up honors deploy.replicas, including zero",
			ComposeCmd("up", "-d"),
			ServiceScale("back", 1),
			ServiceScale("db", 1),
			ServiceScale("front", 2),
			ServiceNotCreated("dbadmin")).
		Step("scale up a zero-replica service",
			ComposeCmd("scale", "dbadmin=2"),
			ServiceScale("dbadmin", 2)).
		Step("scale two services at once",
			ComposeCmd("scale", "front=3", "back=2"),
			ServiceScale("front", 3),
			ServiceScale("back", 2)).
		Step("scale down keeps the surviving replica untouched",
			ComposeCmd("scale", "dbadmin=1"),
			ServiceScale("dbadmin", 1)).
		Step("scale to zero removes every replica",
			ComposeCmd("scale", "dbadmin=0"),
			ServiceNotCreated("dbadmin")).
		Step("scale down two services at once",
			ComposeCmd("scale", "front=2", "back=1"),
			ReplicaNumbers("front", 1, 2),
			ServiceScale("back", 1))
}

func TestScaleWithDepsCases(t *testing.T) {
	NewScenario(t, "scale --no-deps must leave dependencies alone, while a plain scale reconciles them to the model").
		Step("up with an explicit dependency scale",
			ComposeCmd("up", "-d", "--scale", "db=2"),
			ServiceScale("db", 2)).
		Step("scale --no-deps does not touch the dependency's replicas",
			ComposeCmd("scale", "--no-deps", "back=2"),
			ServiceScale("back", 2),
			ServiceScale("db", 2)).
		Step("scale without --no-deps resets the dependency to the model's count",
			ComposeCmd("scale", "back=2"),
			ServiceScale("back", 2),
			ServiceScale("db", 1))
}

func TestScaleUpAndDownPreserveContainerNumber(t *testing.T) {
	NewScenario(t, "scaling down then up must reuse replica numbers instead of drifting").
		Step("up creates replicas #1 and #2",
			ComposeCmd("up", "-d", "--scale", "db=2", "db"),
			ReplicaNumbers("db", 1, 2)).
		Step("scale down removes replica #2",
			ComposeCmd("up", "-d", "--scale", "db=1", "db"),
			ReplicaNumbers("db", 1)).
		Step("scale up restores replica #2",
			ComposeCmd("up", "-d", "--scale", "db=2", "db"),
			ReplicaNumbers("db", 1, 2))
}

func TestScaleDownRemovesObsolete(t *testing.T) {
	NewScenario(t, "scale down must keep the up-to-date replicas, not the lowest numbers").
		Step("up creates replica #1",
			ComposeCmd("up", "-d", "db"),
			ReplicaNumbers("db", 1)).
		Step("scale up with a changed config adds replica #2",
			ComposeCmd("up", "-d", "--scale", "db=2", "db").WithEnv("MAYBE=value"),
			ReplicaNumbers("db", 1, 2)).
		Step("scale down under the same config keeps replica #1",
			ComposeCmd("up", "-d", "--scale", "db=1", "db").WithEnv("MAYBE=value"),
			ReplicaNumbers("db", 1))
}

func TestScaleDownNoRecreate(t *testing.T) {
	s := NewScenario(t, "up --no-recreate must scale up with fresh replicas and scale down by dropping the stale ones")
	s.Defer(DockerCmd("image", "rm", "-f", s.Project()+"-test")).
		Step("build and start two replicas of the first image",
			ComposeCmd("build", "--build-arg", "FOO=test")).
		Step("up starts replicas #1 and #2",
			ComposeCmd("up", "-d", "--scale", "test=2"),
			ReplicaNumbers("test", 1, 2)).
		Step("rebuild changes the image the service should run",
			ComposeCmd("build", "--build-arg", "FOO=updated")).
		Step("up --no-recreate adds fresh replicas without touching stale ones",
			ComposeCmd("up", "-d", "--scale", "test=4", "--no-recreate"),
			ReplicaNumbers("test", 1, 2, 3, 4)).
		Step("scale down drops the stale replicas and keeps the up-to-date ones",
			ComposeCmd("up", "-d", "--scale", "test=2"),
			ReplicaNumbers("test", 3, 4))
}
