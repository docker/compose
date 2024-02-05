/*
   Copyright 2024 Docker Compose CLI authors

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

package tracing

import (
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/stretchr/testify/require"
)

func TestProjectHash(t *testing.T) {
	projA := &types.Project{
		Name:       "fake-proj",
		WorkingDir: "/tmp",
		Services: map[string]types.ServiceConfig{
			"foo": {Image: "fake-image"},
		},
		DisabledServices: map[string]types.ServiceConfig{
			"bar": {Image: "diff-image"},
		},
	}
	projB := &types.Project{
		Name:       "fake-proj",
		WorkingDir: "/tmp",
		Services: map[string]types.ServiceConfig{
			"foo": {Image: "fake-image"},
			"bar": {Image: "diff-image"},
		},
	}
	projC := &types.Project{
		Name:       "fake-proj",
		WorkingDir: "/tmp",
		Services: map[string]types.ServiceConfig{
			"foo": {Image: "fake-image"},
			"bar": {Image: "diff-image"},
			"baz": {Image: "yet-another-image"},
		},
	}

	hashA, ok := projectHash(projA)
	require.True(t, ok)
	require.NotEmpty(t, hashA)
	hashB, ok := projectHash(projB)
	require.True(t, ok)
	require.NotEmpty(t, hashB)
	require.Equal(t, hashA, hashB)

	hashC, ok := projectHash(projC)
	require.True(t, ok)
	require.NotEmpty(t, hashC)
	require.NotEqual(t, hashC, hashA)
}
