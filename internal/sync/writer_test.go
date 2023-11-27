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

package sync

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLossyMultiWriter(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	const count = 5
	readers := make([]*bufReader, count)
	writers := make([]*io.PipeWriter, count)
	for i := 0; i < count; i++ {
		r, w := io.Pipe()
		readers[i] = newBufReader(ctx, r)
		writers[i] = w
	}

	w := newLossyMultiWriter(writers...)
	t.Cleanup(w.Close)
	n, err := w.Write([]byte("hello world"))
	require.Equal(t, 11, n)
	require.NoError(t, err)
	for i := range readers {
		readers[i].waitForWrite(t)
		require.Equal(t, "hello world", string(readers[i].contents()))
		readers[i].reset()
	}

	// even if a writer fails (in this case simulated by closing the receiving end of the pipe),
	// write operations should continue to return nil error but the writer should be closed
	// with an error
	const failIndex = 3
	require.NoError(t, readers[failIndex].r.CloseWithError(errors.New("oh no")))
	n, err = w.Write([]byte("hello"))
	require.Equal(t, 5, n)
	require.NoError(t, err)
	for i := range readers {
		readers[i].waitForWrite(t)
		if i == failIndex {
			err := readers[i].error()
			require.EqualError(t, err, "io: read/write on closed pipe")
			require.Empty(t, readers[i].contents())
		} else {
			require.Equal(t, "hello", string(readers[i].contents()))
		}
	}

	// perform another write, verify there's still no errors
	n, err = w.Write([]byte(" world"))
	require.Equal(t, 6, n)
	require.NoError(t, err)
}

type bufReader struct {
	ctx       context.Context
	r         *io.PipeReader
	mu        sync.Mutex
	err       error
	data      []byte
	writeSync chan struct{}
}

func newBufReader(ctx context.Context, r *io.PipeReader) *bufReader {
	b := &bufReader{
		ctx:       ctx,
		r:         r,
		writeSync: make(chan struct{}),
	}
	go b.consume()
	return b
}

func (b *bufReader) waitForWrite(t testing.TB) {
	t.Helper()
	select {
	case <-b.writeSync:
		return
	case <-time.After(50 * time.Millisecond):
		t.Fatal("timed out waiting for write")
	}
}

func (b *bufReader) consume() {
	defer close(b.writeSync)
	for {
		buf := make([]byte, 512)
		n, err := b.r.Read(buf)
		if n != 0 {
			b.mu.Lock()
			b.data = append(b.data, buf[:n]...)
			b.mu.Unlock()
		}
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			b.mu.Lock()
			b.err = err
			b.mu.Unlock()
			return
		}
		// prevent goroutine leak, tie lifetime to the test
		select {
		case b.writeSync <- struct{}{}:
		case <-b.ctx.Done():
			return
		}
	}
}

func (b *bufReader) contents() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.data
}

func (b *bufReader) reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = nil
}

func (b *bufReader) error() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.err
}
