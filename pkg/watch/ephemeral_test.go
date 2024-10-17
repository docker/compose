/*
Copyright 2023 Docker Compose CLI authors

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
package watch_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/docker/compose/v2/pkg/watch"
)

func TestEphemeralPathMatcher(t *testing.T) {
	ignored := []string{
		".file.txt.swp",
		"/path/file.txt~",
		"/home/moby/proj/.idea/modules.xml",
		".#file.txt",
		"#file.txt#",
		"/dir/.file.txt.kate-swp",
		"/go/app/1234-go-tmp-umask",
	}
	matcher := watch.EphemeralPathMatcher()
	for _, p := range ignored {
		ok, err := matcher.Matches(p)
		require.NoErrorf(t, err, "Matching %s", p)
		assert.Truef(t, ok, "Path %s should have matched", p)
	}

	const includedPath = "normal.txt"
	ok, err := matcher.Matches(includedPath)
	require.NoErrorf(t, err, "Matching %s", includedPath)
	assert.Falsef(t, ok, "Path %s should NOT have matched", includedPath)
}
