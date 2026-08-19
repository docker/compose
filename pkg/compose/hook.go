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
	// The output of a hook is the only explanation of why it failed, so keep the
	// tail of it even when no listener is attached: the exec used to run
	// unattached in that case, the daemon discarded both streams, and the caller
	// was left with a bare exit code. Only surfaced on failure, and bounded.
	tail := newOutputTail(hookOutputTailLines, hookOutputTailBytes)

	var out io.Writer = tail

	if listener != nil {
		wOut := utils.GetWriter(func(line string) {
			listener(api.ContainerEvent{
				Type:    api.HookEventLog,
				Source:  getContainerNameWithoutProject(ctr) + " ->",
				ID:      ctr.ID,
				Service: service.Name,
				Line:    line,
			})
		})
		defer wOut.Close() //nolint:errcheck

		out = io.MultiWriter(wOut, tail)
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

	if service.Tty {
		_, err = io.Copy(out, attach.Reader)
	} else {
		_, err = stdcopy.StdCopy(out, out, attach.Reader)
	}
	if err != nil {
		return err
	}

	inspected, err := s.apiClient().ExecInspect(ctx, exec.ID, client.ExecInspectOptions{})
	if err != nil {
		return err
	}
	if inspected.ExitCode != 0 {
		if output := tail.String(); output != "" {
			return fmt.Errorf("%s hook exited with status %d: %s", service.Name, inspected.ExitCode, output)
		}

		return fmt.Errorf("%s hook exited with status %d", service.Name, inspected.ExitCode)
	}
	return nil
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
	for _, b := range p {
		if b != '\n' {
			// Cap the line being assembled, so a hook printing megabytes without
			// a newline cannot grow this buffer without bound.
			if t.partial.Len() < t.maxBytes {
				t.partial.WriteByte(b)
			}

			continue
		}

		t.pushLine(t.partial.String())
		t.partial.Reset()
	}

	return len(p), nil
}

// pushLine appends a line and drops the oldest one when the limit is reached.
func (t *outputTail) pushLine(line string) {
	line = strings.TrimRight(line, "\r")
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
	if partial := strings.TrimSpace(t.partial.String()); partial != "" {
		lines = append(append([]string{}, lines...), partial)
	}

	output := strings.TrimSpace(strings.Join(lines, "\n"))
	if len(output) > t.maxBytes {
		output = output[len(output)-t.maxBytes:]
	}

	return output
}
