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
	}, 2*time.Second, 20*time.Millisecond,
		"Buffer did not contain %q\n============\n%s\n============",
		v, &bufContents)
}
