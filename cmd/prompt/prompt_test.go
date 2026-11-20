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
	"bufio"
	"bytes"
	"io"
	"os"
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
	ptmx, user := newTestUser(t)

	done := make(chan struct {
		answer bool
		err    error
	}, 1)

	go func() {
		answer, err := user.Confirm("Continue?", false)
		done <- struct {
			answer bool
			err    error
		}{answer, err}
	}()

	readUntil(t, ptmx, "Continue? [y/N]: ")

	_, err := ptmx.Write([]byte("maybe\r"))
	assert.NilError(t, err)

	readUntil(t, ptmx, "Continue? [y/N]: ")

	// Surrounding whitespace is ignored.
	_, err = ptmx.Write([]byte(" y \r"))
	assert.NilError(t, err)

	select {
	case result := <-done:
		assert.NilError(t, result.err)
		assert.Assert(t, result.answer)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for prompt to return")
	}
}

// TestUserConfirmDefault verifies that an empty answer selects the default.
func TestUserConfirmDefault(t *testing.T) {
	ptmx, user := newTestUser(t)

	done := make(chan struct {
		answer bool
		err    error
	}, 1)

	go func() {
		answer, err := user.Confirm("Continue?", true)
		done <- struct {
			answer bool
			err    error
		}{answer, err}
	}()

	readUntil(t, ptmx, "Continue? [Y/n]: ")

	_, err := ptmx.Write([]byte{'\r'})
	assert.NilError(t, err)

	select {
	case result := <-done:
		assert.NilError(t, result.err)
		assert.Assert(t, result.answer)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for prompt to return")
	}
}

// TestUserConfirmSequential verifies consecutive confirmations consume
// one line of input at a time.
func TestUserConfirmSequential(t *testing.T) {
	ptmx, user := newTestUser(t)

	done := make(chan struct {
		first, second bool
		err           error
	}, 1)
	go func() {
		first, err := user.Confirm("first?", false)
		if err != nil {
			done <- struct {
				first, second bool
				err           error
			}{err: err}
			return
		}
		second, err := user.Confirm("second?", true)
		done <- struct {
			first, second bool
			err           error
		}{first, second, err}
	}()

	readUntil(t, ptmx, "first? [y/N]: ")
	_, err := ptmx.Write([]byte("y\r"))
	assert.NilError(t, err)

	readUntil(t, ptmx, "second? [Y/n]: ")
	_, err = ptmx.Write([]byte("n\r"))
	assert.NilError(t, err)

	select {
	case result := <-done:
		assert.NilError(t, result.err)
		assert.Assert(t, result.first)
		assert.Assert(t, !result.second)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for prompts to return")
	}
}

// Arrow keys arrive as raw escape sequences in raw mode: the whole sequence
// must be swallowed — neither echoed nor taken as answer characters.
func TestUserConfirmIgnoresArrowKeys(t *testing.T) {
	ptmx, user := newTestUser(t)

	done := make(chan struct {
		answer bool
		err    error
	}, 1)
	go func() {
		answer, err := user.Confirm("Continue?", false)
		done <- struct {
			answer bool
			err    error
		}{answer, err}
	}()

	readUntil(t, ptmx, "Continue? [y/N]: ")

	// Up arrow (CSI), Home in application mode (SS3), then a real answer.
	_, err := ptmx.Write([]byte("\x1b[A\x1bOH y\r"))
	assert.NilError(t, err)

	select {
	case result := <-done:
		assert.NilError(t, result.err)
		assert.Assert(t, result.answer, "the arrow-key bytes must not corrupt the answer")
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for prompt to return")
	}
}

// Both backspace encodings erase: DEL (most terminals) and ^H (some
// terminals, legacy Windows console).
func TestReadLineBackspaceVariants(t *testing.T) {
	var stdout bytes.Buffer
	line, err := readLine(bufio.NewReader(strings.NewReader("nx\x7fy\x08o\r")), &stdout)
	assert.NilError(t, err)
	assert.Equal(t, line, "no")
}

func TestReadLineInterrupt(t *testing.T) {
	var stdout bytes.Buffer

	_, err := readLine(
		bufio.NewReader(strings.NewReader("\x03")),
		&stdout,
	)

	assert.ErrorIs(t, err, ErrInterrupt)
	assert.Equal(t, stdout.String(), "\r\n")
}

func newTestUser(t *testing.T) (*os.File, User) {
	t.Helper()

	ptmx, tty, err := pty.Open()
	assert.NilError(t, err)
	t.Cleanup(func() {
		_ = tty.Close()
		_ = ptmx.Close()
	})

	stdin := streams.NewIn(tty)
	return ptmx, User{stdin: stdin, reader: bufio.NewReader(stdin), stdout: streams.NewOut(tty)}
}

// readUntil reads until the expected string is observed.
func readUntil(t *testing.T, r io.Reader, want string) {
	t.Helper()

	type result struct {
		got string
		err error
	}

	done := make(chan result, 1)
	go func() {
		var got strings.Builder
		buf := make([]byte, 64)
		for !strings.Contains(got.String(), want) {
			n, err := r.Read(buf)
			if err != nil {
				done <- result{got: got.String(), err: err}
				return
			}
			got.Write(buf[:n])
		}
		done <- result{got: got.String()}
	}()

	select {
	case res := <-done:
		assert.NilError(t, res.err, "reading until %q; got %q", want, res.got)
	case <-time.After(time.Second):
		t.Fatalf("timed out reading until %q", want)
	}
}
