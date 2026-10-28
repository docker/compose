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
	"path/filepath"
	"testing"
)

func TestCopy(t *testing.T) {
	s := NewScenario(t, "cp must target all replicas by default, one replica with --index, and copy both ways")
	dir := s.Dir()
	s.Step("up starts five replicas",
		ComposeCmd("up", "--scale", "nginx=5", "-d"),
		ServiceScale("nginx", 5)).
		Step("cp to the service reaches every replica by default",
			ComposeCmd("cp", filepath.Join(dir, "cp-me.txt"), "nginx:/tmp/default.txt")).
		Step("replica #1 got the file",
			DockerCmd("exec", s.Project()+"-nginx-1", "cat", "/tmp/default.txt"),
			OutputContains("hello world")).
		Step("replica #3 got the file too",
			DockerCmd("exec", s.Project()+"-nginx-3", "cat", "/tmp/default.txt"),
			OutputContains("hello world")).
		Step("cp --index targets a single replica",
			ComposeCmd("cp", "--index=3", filepath.Join(dir, "cp-me.txt"), "nginx:/tmp/indexed.txt")).
		Step("the indexed replica got the file",
			DockerCmd("exec", s.Project()+"-nginx-3", "cat", "/tmp/indexed.txt"),
			OutputContains("hello world")).
		Step("the other replicas did not",
			DockerCmd("exec", s.Project()+"-nginx-2", "cat", "/tmp/indexed.txt").MayFail(),
			ExitCode(1)).
		Step("cp from the service reads from replica #1 by default",
			ComposeCmd("cp", "nginx:/tmp/default.txt", filepath.Join(dir, "from-default.txt")),
			FileContains(filepath.Join(dir, "from-default.txt"), "hello world")).
		Step("cp --index reads from the chosen replica",
			ComposeCmd("cp", "--index=3", "nginx:/tmp/indexed.txt", filepath.Join(dir, "from-indexed.txt")),
			FileContains(filepath.Join(dir, "from-indexed.txt"), "hello world")).
		Step("cp copies a folder into the container",
			ComposeCmd("cp", filepath.Join(dir, "cp-folder"), "nginx:/tmp")).
		Step("the folder's content is in the container",
			DockerCmd("exec", s.Project()+"-nginx-1", "cat", "/tmp/cp-folder/cp-me.txt"),
			OutputContains("hello world from folder")).
		Step("cp copies a folder out of the container",
			ComposeCmd("cp", "nginx:/tmp/cp-folder", filepath.Join(dir, "cp-folder2")),
			FileContains(filepath.Join(dir, "cp-folder2", "cp-me.txt"), "hello world from folder"))
}
