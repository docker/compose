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

const secretsProject = `
-- compose.yaml --
include:
  - path: child/compose.yaml
    env_file:
      - secret.env

services:
  foo:
    image: alpine
    secrets:
      - source: secret
        target: bar
        uid: "1005"
        gid: "1005"
        mode: 0440
    command: cat /run/secrets/bar

secrets:
  secret:
    environment: SECRET
-- secret.env --
MY_SECRET='this-is-secret'
-- child/compose.yaml --
services:
  included:
    image: alpine
    secrets:
      - my-secret
    command: cat /run/secrets/my-secret

secrets:
  my-secret:
    environment: 'MY_SECRET'
`

func TestSecretFromEnv(t *testing.T) {
	NewScenario(t, "an environment-sourced secret must be mounted with its content, ownership and mode").
		Files(secretsProject).
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
		Files(secretsProject).
		Step("the included service reads the secret defined by the include's env_file",
			ComposeCmd("run", "included"),
			OutputContains("this-is-secret"))
}
