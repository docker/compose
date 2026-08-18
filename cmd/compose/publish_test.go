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

	"gotest.tools/v3/assert"
)

func TestParseAnnotations(t *testing.T) {
	t.Run("valid annotation", func(t *testing.T) {
		result, err := parseAnnotations([]string{"foo=bar"})
		assert.NilError(t, err)
		assert.DeepEqual(t, result, map[string]string{"foo": "bar"})
	})

	t.Run("invalid annotation missing equals sign", func(t *testing.T) {
		_, err := parseAnnotations([]string{"foobar"})
		assert.Error(t, err, `invalid annotation "foobar": expected format key=value`)
	})

	t.Run("empty slice", func(t *testing.T) {
		result, err := parseAnnotations([]string{})
		assert.NilError(t, err)
		assert.Check(t, result == nil)
	})
}
