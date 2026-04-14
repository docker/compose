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

package formatter

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestOSC8Link(t *testing.T) {
	disableAnsi = false
	t.Cleanup(func() { disableAnsi = false })

	got := OSC8Link("http://example.com", "click here")
	want := "\x1b]8;;http://example.com\x1b\\\x1b[4mclick here\x1b[24m\x1b]8;;\x1b\\"
	assert.Equal(t, got, want)
}

func TestOSC8Link_AnsiDisabled(t *testing.T) {
	disableAnsi = true
	t.Cleanup(func() { disableAnsi = false })

	got := OSC8Link("http://example.com", "click here")
	assert.Equal(t, got, "click here")
}

func TestOSC8Link_URLAsDisplayText(t *testing.T) {
	disableAnsi = false
	t.Cleanup(func() { disableAnsi = false })

	url := "docker-desktop://dashboard/logs"
	got := OSC8Link(url, url)
	want := "\x1b]8;;docker-desktop://dashboard/logs\x1b\\\x1b[4mdocker-desktop://dashboard/logs\x1b[24m\x1b]8;;\x1b\\"
	assert.Equal(t, got, want)
}
