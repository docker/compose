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

package display

import (
	"bytes"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
)

func TestPlain_DryRun(t *testing.T) {
	var out bytes.Buffer
	ep := Plain(&out, true)
	ep.On(api.Resource{ID: "service1", Text: api.StatusCreating})

	assert.Assert(t, strings.Contains(out.String(), DRYRUN_PREFIX))
}

func TestPlain_NotDryRun(t *testing.T) {
	var out bytes.Buffer
	ep := Plain(&out, false)
	ep.On(api.Resource{ID: "service1", Text: api.StatusCreating})

	assert.Assert(t, !strings.Contains(out.String(), DRYRUN_PREFIX))
}
