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
	"os"
	"path/filepath"
	"testing"
)

func TestSecretFromEnv(t *testing.T) {
	NewScenario(t, "an environment-sourced secret must be mounted with its content, ownership and mode").
		Env("SECRET=BAR").
		Step("the service reads the secret's content from the mounted file",
			ComposeCmd("run", "foo"),
			OutputContains("BAR")).
		Step("the mounted secret carries the declared uid, gid and mode",
			ComposeCmd("run", "foo", "ls", "-al", "/var/run/secrets/bar"),
			OutputContains("-r--r-----    1 1005     1005"))
}

func TestSecretFromInclude(t *testing.T) {
	NewScenario(t, "a secret declared by an included project must resolve from the include's env_file").
		Step("the included service reads the secret defined by the include's env_file",
			ComposeCmd("run", "included"),
			OutputContains("this-is-secret"))
}

// https://github.com/docker/compose/issues/11867
// secrets.*.driver: copy must deliver a one-time snapshot of the CLIENT's
// local file, read and copied into the container — not a live bind mount
// from a path on the Docker host, which breaks entirely once the daemon is
// remote. Editing the client's file after `up` must not reach the running
// container: that's the behavioral difference from the default bind mount,
// and the whole point of opting in.
func TestSecretCopyDriver(t *testing.T) {
	s := NewScenario(t, "secrets.*.driver: copy must copy the client's file once, not bind-mount it live")
	s.Step("up copies the secret's original content into the container",
		ComposeCmd("up", "-d"),
		ServiceState("test", "running")).
		Step("the container holds the original content",
			ComposeCmd("exec", "test", "cat", "/run/secrets/db_password"),
			StdoutContains("original-secret"))

	err := os.WriteFile(filepath.Join(s.Dir(), "secret.txt"), []byte("changed-secret"), 0o644)
	if err != nil {
		t.Fatalf("updating client secret file: %v", err)
	}

	s.Step("the running container's copy is unaffected by the client-side edit",
		ComposeCmd("exec", "test", "cat", "/run/secrets/db_password"),
		StdoutContains("original-secret"),
		OutputNotContains("changed-secret"))
}
