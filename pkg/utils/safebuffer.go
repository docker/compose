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

package utils

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// SafeBuffer is a thread safe version of bytes.Buffer
type SafeBuffer struct {
	m sync.RWMutex
	b bytes.Buffer
}

// Read is a thread safe version of bytes.Buffer::Read
func (b *SafeBuffer) Read(p []byte) (n int, err error) {
	b.m.RLock()
	defer b.m.RUnlock()
	return b.b.Read(p)
}

// Write is a thread safe version of bytes.Buffer::Write
func (b *SafeBuffer) Write(p []byte) (n int, err error) {
	b.m.Lock()
	defer b.m.Unlock()
	return b.b.Write(p)
}

// String is a thread safe version of bytes.Buffer::String
func (b *SafeBuffer) String() string {
	b.m.RLock()
	defer b.m.RUnlock()
	return b.b.String()
}

// Bytes is a thread safe version of bytes.Buffer::Bytes
func (b *SafeBuffer) Bytes() []byte {
	b.m.RLock()
	defer b.m.RUnlock()
	return b.b.Bytes()
}

// RequireEventuallyContains is a thread safe eventual checker for the buffer content
func (b *SafeBuffer) RequireEventuallyContains(t testing.TB, v string) {
	t.Helper()
	var bufContents strings.Builder
	require.Eventuallyf(t, func() bool {
		b.m.Lock()
		defer b.m.Unlock()
		if _, err := b.b.WriteTo(&bufContents); err != nil {
			require.FailNowf(t, "Failed to copy from buffer",
				"Error: %v", err)
		}
		return strings.Contains(bufContents.String(), v)
	}, 2*time.Second, 20*time.Millisecond,
		"Buffer did not contain %q\n============\n%s\n============",
		v, &bufContents)
}
