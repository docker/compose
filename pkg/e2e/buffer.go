/*
   Copyright 2022 Docker Compose CLI authors

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

package e2e

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *lockedBuffer) Read(p []byte) (n int, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Read(p)
}

func (l *lockedBuffer) Write(p []byte) (n int, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

func (l *lockedBuffer) RequireEventuallyContains(t testing.TB, v string) {
	t.Helper()
	var bufContents strings.Builder
	require.Eventuallyf(t, func() bool {
		l.mu.Lock()
		defer l.mu.Unlock()
		if _, err := l.buf.WriteTo(&bufContents); err != nil {
			require.FailNowf(t, "Failed to copy from buffer",
				"Error: %v", err)
		}
		return strings.Contains(bufContents.String(), v)
	}, 5*time.Second, 20*time.Millisecond,
		"Buffer did not contain %q\n============\n%s\n============",
		v, &bufContents)
}
