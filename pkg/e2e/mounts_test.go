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
	"time"
)

func TestMountsBindCreateSrc(t *testing.T) {
	s := NewScenario(t, "a bind mount of `mounts` must create its missing host source when it sets create_src")

	s.Step("up mounts the source it had to create",
		ComposeCmd("up", "-d", "--wait").Within(60*time.Second),
		ServiceState("app", "running"),
		BindMountSource("app", "/data", filepath.Join(s.Dir(), "data")))
}

func TestMountsBindSourceMustExist(t *testing.T) {
	s := NewScenario(t, "a bind mount of `mounts` must not create its host source unless it says so, as `volumes` does")
	missing := filepath.Join(s.Dir(), "missing")

	s.Step("up fails on the missing source and leaves the host untouched",
		ComposeCmd("up", "-d").MayFail(),
		ExitCode(1),
		FileAbsent(missing))
}

func TestMountsVolumeDriverOpts(t *testing.T) {
	s := NewScenario(t, "a volume mount must create its volume with the driver options of the mount")

	s.Step("up mounts an anonymous volume backed by the options of the mount",
		ComposeCmd("up", "-d", "--wait").Within(60*time.Second),
		ServiceState("app", "running"),
		ExecOutputContains("app", "grep ' /scratch ' /proc/mounts", "tmpfs /scratch tmpfs"))
}
