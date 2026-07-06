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

package prompt

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/docker/cli/cli/streams"
	"gotest.tools/v3/assert"
)

// TestPipeConfirmSequential verifies consecutive piped confirmations consume
// one line of input at a time.
func TestPipeConfirmSequential(t *testing.T) {
	var stdout bytes.Buffer
	pipe := Pipe{
		stdin:  strings.NewReader("y\nn\n"),
		stdout: &stdout,
	}

	got, err := pipe.Confirm("first? ", false)
	assert.NilError(t, err)
	assert.Assert(t, got)

	got, err = pipe.Confirm("second? ", true)
	assert.NilError(t, err)
	assert.Assert(t, !got)
}

// TestUserConfirm verifies that an interactive terminal confirmation returns
// the expected answer and retries invalid input.
func TestUserConfirm(t *testing.T) {
	ptmx, tty, err := pty.Open()
	assert.NilError(t, err)
	t.Cleanup(func() {
		_ = tty.Close()
		_ = ptmx.Close()
	})

	user := User{
		stdin:  streamsFileReader{streams.NewIn(tty)},
		stdout: streamsFileWriter{streams.NewOut(tty)},
	}

	done := make(chan struct {
		answer bool
		err    error
	}, 1)

	go func() {
		answer, err := user.Confirm("Continue? ", false)
		done <- struct {
			answer bool
			err    error
		}{answer, err}
	}()

	readUntil(t, ptmx, "Continue? ")

	_, err = ptmx.Write([]byte("maybe\r"))
	assert.NilError(t, err)

	readUntil(t, ptmx, "Continue? ")

	_, err = ptmx.Write([]byte("y\r"))
	assert.NilError(t, err)

	result := <-done
	assert.NilError(t, result.err)
	assert.Assert(t, result.answer)
}

// TestUserConfirmInterrupt verifies that Ctrl+C interrupts an interactive
// terminal confirmation.
func TestUserConfirmInterrupt(t *testing.T) {
	ptmx, tty, err := pty.Open()
	assert.NilError(t, err)
	t.Cleanup(func() {
		_ = tty.Close()
		_ = ptmx.Close()
	})

	user := User{
		stdin:  streamsFileReader{streams.NewIn(tty)},
		stdout: streamsFileWriter{streams.NewOut(tty)},
	}

	done := make(chan error, 1)
	go func() {
		_, err := user.Confirm("Continue? ", false)
		done <- err
	}()

	readUntil(t, ptmx, "Continue? ")

	_, err = ptmx.Write([]byte{3}) // Ctrl+C
	assert.NilError(t, err)

	select {
	case err := <-done:
		assert.ErrorIs(t, err, io.EOF)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for prompt to return")
	}
}

// readUntil reads until the expected string is observed.
func readUntil(t *testing.T, r io.Reader, want string) {
	t.Helper()

	var got strings.Builder
	buf := make([]byte, 64)
	for !strings.Contains(got.String(), want) {
		n, err := r.Read(buf)
		assert.NilError(t, err, "reading until %q; got %q", want, got.String())
		got.Write(buf[:n])
	}
}
