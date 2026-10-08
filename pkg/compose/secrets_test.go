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

package compose

import (
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/sirupsen/logrus"
	logrustest "github.com/sirupsen/logrus/hooks/test"
	"gotest.tools/v3/assert"
)

func TestWarnIgnoredFileReferences(t *testing.T) {
	mode := types.FileMode(0o400)
	zero := 0
	overrides := func(source string) types.ServiceConfigObjConfig {
		return types.ServiceConfigObjConfig{Source: source, UID: "1000", GID: "1000", Mode: &mode}
	}
	project := &types.Project{
		Services: types.Services{
			"web": {
				Name: "web",
				ContainerSpec: types.ContainerSpec{
					Configs: []types.ServiceConfigObjConfig{overrides("file-cfg"), overrides("inline-cfg"), overrides("env-cfg"), overrides("external-cfg"), overrides("driver-cfg"), {Source: "file-cfg"}},
					Secrets: []types.ServiceSecretConfig{{Source: "file-secret", UID: "1000"}, {Source: "env-secret", UID: "1000", GID: "1000", Mode: &mode}},
				},
			},
			"other": {
				Name:          "other",
				ContainerSpec: types.ContainerSpec{Configs: []types.ServiceConfigObjConfig{overrides("file-cfg")}},
			},
			// scaled to zero: no container, nothing to warn about
			"scaled-down": {
				Name:          "scaled-down",
				Scale:         &zero,
				ContainerSpec: types.ContainerSpec{Configs: []types.ServiceConfigObjConfig{overrides("file-cfg")}},
			},
			// a reference without source reaches the zero-value object
			"anonymous": {
				Name:          "anonymous",
				ContainerSpec: types.ContainerSpec{Configs: []types.ServiceConfigObjConfig{{UID: "1000"}}},
			},
		},
		Configs: types.Configs{
			"file-cfg":   {File: "./c.txt"},
			"inline-cfg": {Content: "hello"},
			"env-cfg":    {Environment: "FOO"},
			// rejected when mounts are built: an error, not a warning
			"external-cfg": {External: true},
			"driver-cfg":   {Driver: "custom"},
		},
		Secrets: types.Secrets{
			"file-secret": {File: "./s.txt"},
			"env-secret":  {Environment: "BAR"},
		},
	}

	collect := func(services []string) []string {
		hook := logrustest.NewGlobal()
		defer hook.Reset()
		warnIgnoredFileReferences(project, services)
		var msgs []string
		for _, e := range hook.AllEntries() {
			assert.Equal(t, e.Level, logrus.WarnLevel)
			msgs = append(msgs, e.Message)
		}
		return msgs
	}

	const ignored = " is not supported outside Swarm mode and will be ignored"
	// only `file:` sources are bind-mounted, so only their overrides are
	// dropped; content and environment sources honor all of them.
	assert.DeepEqual(t, collect([]string{"web", "other", "anonymous", "scaled-down"}), []string{
		`service "anonymous": configs.(anonymous).uid: uid` + ignored,
		`service "other": configs.file-cfg.uid: uid` + ignored,
		`service "other": configs.file-cfg.gid: gid` + ignored,
		`service "other": configs.file-cfg.mode: mode` + ignored,
		`service "web": configs.file-cfg.uid: uid` + ignored,
		`service "web": configs.file-cfg.gid: gid` + ignored,
		`service "web": configs.file-cfg.mode: mode` + ignored,
		`service "web": secrets.file-secret.uid: uid` + ignored,
	})

	// services outside the requested set are not reported.
	assert.Equal(t, len(collect([]string{"other"})), 3)
	assert.Equal(t, len(collect([]string{"unknown"})), 0)
}
