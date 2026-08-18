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
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/utils"
)

const (
	// hookOutputTailLines and hookOutputTailBytes bound how much of a failing
	// hook's output is kept for the error message.
	hookOutputTailLines = 10
	hookOutputTailBytes = 2048
)

func (s *composeService) runHook(ctx context.Context, ctr container.Summary, service types.ServiceConfig, hook types.ServiceHook, listener api.ContainerEventListener) error {
	// Keep stdout and stderr in separate tails so the error message can prefer
	// stderr (which usually holds the real error) over stdout progress noise.
	tailOut := newOutputTail(hookOutputTailLines, hookOutputTailBytes)
	tailErr := newOutputTail(hookOutputTailLines, hookOutputTailBytes)

	// wOut/wErr receive the demultiplexed stdout/stderr from StdCopy. When a
	// listener is attached they also forward each line to it.
	var wOut, wErr io.Writer
	wOut = tailOut
	wErr = tailErr

	if listener != nil {
		source := getContainerNameWithoutProject(ctr) + " ->"
		// stdout and stderr share one listener writer: ContainerEvent has no stream
		// field, so both appear identically in the live display.
		lw := utils.GetWriter(func(line string) {
			listener(api.ContainerEvent{
				Type:    api.HookEventLog,
				Source:  source,
				ID:      ctr.ID,
				Service: service.Name,
				Line:    line,
			})
		})
		defer lw.Close() //nolint:errcheck
		wOut = io.MultiWriter(lw, tailOut)
		wErr = io.MultiWriter(lw, tailErr)
	}

	exec, err := s.apiClient().ExecCreate(ctx, ctr.ID, client.ExecCreateOptions{
		User:         hook.User,
		Privileged:   hook.Privileged,
		Env:          ToMobyEnv(hook.Environment),
		WorkingDir:   hook.WorkingDir,
		Cmd:          hook.Command,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return err
	}

	attachOptions := client.ExecAttachOptions{
		TTY: service.Tty,
	}
	if service.Tty {
		height, width := s.stdout().GetTtySize()
		attachOptions.ConsoleSize = client.ConsoleSize{
			Width:  width,
			Height: height,
		}
	}

	attach, err := s.apiClient().ExecAttach(ctx, exec.ID, attachOptions)
	if err != nil {
		return err
	}
	defer attach.Close()

	// Run the copy in a goroutine so that a context cancellation (Ctrl+C) can
	// interrupt it. Without this the blocked Read inside StdCopy/io.Copy would
	// prevent the first Ctrl+C from taking effect.
	copyDone := make(chan error, 1)
	go func() {
		var cErr error
		if service.Tty {
			_, cErr = io.Copy(wOut, attach.Reader)
		} else {
			_, cErr = stdcopy.StdCopy(wOut, wErr, attach.Reader)
		}
		copyDone <- cErr
	}()

	select {
	case err = <-copyDone:
		if err != nil {
			return err
		}
	case <-ctx.Done():
		// Close the connection to unblock the blocked Read in the copy goroutine,
		// then wait for it to exit before returning so there is no goroutine leak.
		attach.Close()
		<-copyDone
		return ctx.Err()
	}

	inspected, err := s.apiClient().ExecInspect(ctx, exec.ID, client.ExecInspectOptions{})
	if err != nil {
		return err
	}
	if inspected.ExitCode != 0 {
		return hookExitError(service.Name, inspected.ExitCode, tailOut, tailErr)
	}
	return nil
}

// hookExitError builds a hook failure error. Stderr content is preferred over
// stdout because it usually holds the real failure reason rather than progress
// noise. Falls back to stdout when stderr is empty.
func hookExitError(serviceName string, code int, tailOut, tailErr *outputTail) error {
	output := tailErr.String()
	if output == "" {
		output = tailOut.String()
	}
	if output == "" {
		return fmt.Errorf("%s hook exited with status %d", serviceName, code)
	}
	return fmt.Errorf("%s hook exited with status %d: %s", serviceName, code, output)
}

// outputTail keeps the last lines written to it, bounded by a line and a byte
// count. Writes past the limit are accepted and the oldest content is dropped,
// so a chatty hook is never blocked by a full buffer. It is written from a
// single goroutine (stdcopy or io.Copy) and read after that copy returned.
type outputTail struct {
	maxLines int
	maxBytes int
	lines    []string
	partial  strings.Builder
}

func newOutputTail(maxLines, maxBytes int) *outputTail {
	return &outputTail{maxLines: maxLines, maxBytes: maxBytes}
}

func (t *outputTail) Write(p []byte) (int, error) {
	n := len(p)
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		if i < 0 {
			t.appendToPartial(p)
			break
		}
		t.appendToPartial(p[:i])
		// ToValidUTF8 ensures a byte-truncated multi-byte rune does not corrupt
		// the error message.
		t.pushLine(strings.ToValidUTF8(t.partial.String(), ""))
		t.partial.Reset()
		p = p[i+1:]
	}
	return n, nil
}

// appendToPartial appends b to the in-progress partial line, capped at maxBytes
// so a hook printing megabytes without a newline cannot grow the buffer unbounded.
func (t *outputTail) appendToPartial(b []byte) {
	if remaining := t.maxBytes - t.partial.Len(); remaining > 0 {
		if len(b) > remaining {
			b = b[:remaining]
		}
		_, _ = t.partial.Write(b)
	}
}

// pushLine appends a line and drops the oldest one when the limit is reached.
func (t *outputTail) pushLine(line string) {
	// Strip trailing \r (from \r\n sequences). For progress-bar output that uses
	// bare \r to rewind the cursor (curl, apt, pip), keep only the last segment
	// after the final \r so the visible content is preserved rather than the
	// intermediate states.
	line = strings.TrimRight(line, "\r")
	if idx := strings.LastIndex(line, "\r"); idx >= 0 {
		line = line[idx+1:]
	}
	if strings.TrimSpace(line) == "" {
		return
	}

	t.lines = append(t.lines, line)
	if len(t.lines) > t.maxLines {
		t.lines = t.lines[len(t.lines)-t.maxLines:]
	}
}

// String returns the kept output, newest content last, trimmed to maxBytes.
func (t *outputTail) String() string {
	lines := t.lines
	if partial := strings.TrimSpace(strings.ToValidUTF8(t.partial.String(), "")); partial != "" {
		lines = append(lines, partial)
	}

	output := strings.TrimSpace(strings.Join(lines, "\n"))
	if len(output) > t.maxBytes {
		// Byte-slicing may split a multi-byte rune; ToValidUTF8 drops the broken
		// leading sequence rather than emitting a replacement character.
		output = strings.ToValidUTF8(output[len(output)-t.maxBytes:], "")
	}

	return output
}
