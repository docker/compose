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

	"github.com/docker/compose/v5/internal"
	"github.com/hashicorp/go-version"
	"gotest.tools/v3/assert"
)

func TestComposeVersionInitialization(t *testing.T) {
	v, err := version.NewVersion(internal.Version)
	if err != nil {
		assert.Equal(t, "", ComposeVersion, "ComposeVersion should be empty for a non-semver internal version (e.g. 'devel')")
	} else {
		expected := v.Core().String()
		assert.Equal(t, expected, ComposeVersion, "ComposeVersion should be the core of internal.Version")
	}
}
