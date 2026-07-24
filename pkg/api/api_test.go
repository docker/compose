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

package api

import (
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"gotest.tools/v3/assert"
)

func TestRunOptionsEnvironmentMap(t *testing.T) {
	opts := RunOptions{
		Environment: []string{
			"FOO=BAR",
			"ZOT=",
			"QIX",
		},
	}
	env := types.NewMappingWithEquals(opts.Environment)
	assert.Equal(t, *env["FOO"], "BAR")
	assert.Equal(t, *env["ZOT"], "")
	assert.Check(t, env["QIX"] == nil)
}

func TestGetImageNamesForServiceIncludesPreStartHooks(t *testing.T) {
	service := types.ServiceConfig{
		Name:  "demo",
		Image: "alpine:3.20",
		PreStart: []types.ServiceHook{
			{Image: "alpine:3.20"},
			{Image: "alpine:3.19"},
			{Image: "alpine:3.19"},
			{},
		},
		PostStart: []types.ServiceHook{{Image: "unused-post-start"}},
		PreStop:   []types.ServiceHook{{Image: "unused-pre-stop"}},
	}

	assert.DeepEqual(t, GetImageNamesForService(service, "hooktest"), []string{"alpine:3.20", "alpine:3.19"})
}
