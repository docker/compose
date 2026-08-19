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

func TestParseDetailsLine(t *testing.T) {
	t.Run("hook line", func(t *testing.T) {
		l := parseDetailsLine("com.docker.compose.hook=post_start,com.docker.compose.service=db,exec_id=abc123 SQL ready", false)
		assert.Equal(t, l.Hook, "post_start")
		assert.Equal(t, l.Payload, "SQL ready")
	})

	t.Run("plain container line carries an empty attribute block", func(t *testing.T) {
		l := parseDetailsLine(" hello world", false)
		assert.Equal(t, l.Hook, "")
		assert.Equal(t, l.Payload, "hello world")
	})

	t.Run("line with non-hook attributes", func(t *testing.T) {
		l := parseDetailsLine("env=prod hello", false)
		assert.Equal(t, l.Hook, "")
		assert.Equal(t, l.Payload, "hello")
	})

	t.Run("timestamps precede the attribute block and stay on the payload", func(t *testing.T) {
		l := parseDetailsLine("2026-08-19T10:00:00.000000000Z com.docker.compose.hook=pre_stop bye", true)
		assert.Equal(t, l.Hook, "pre_stop")
		assert.Equal(t, l.Payload, "2026-08-19T10:00:00.000000000Z bye")
	})

	t.Run("url-encoded value", func(t *testing.T) {
		l := parseDetailsLine("com.docker.compose.hook=post%5Fstart go", false)
		assert.Equal(t, l.Hook, "post_start")
	})
}
