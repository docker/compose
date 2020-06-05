/*
	Copyright (c) 2020 Docker Inc.

	Permission is hereby granted, free of charge, to any person
	obtaining a copy of this software and associated documentation
	files (the "Software"), to deal in the Software without
	restriction, including without limitation the rights to use, copy,
	modify, merge, publish, distribute, sublicense, and/or sell copies
	of the Software, and to permit persons to whom the Software is
	furnished to do so, subject to the following conditions:

	The above copyright notice and this permission notice shall be
	included in all copies or substantial portions of the Software.

	THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
	EXPRESS OR IMPLIED,
	INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
	FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
	IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT
	HOLDERS BE LIABLE FOR ANY CLAIM,
	DAMAGES OR OTHER LIABILITY,
	WHETHER IN AN ACTION OF CONTRACT,
	TORT OR OTHERWISE,
	ARISING FROM, OUT OF OR IN CONNECTION WITH
	THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*/

package framework

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/onsi/gomega"
	log "github.com/sirupsen/logrus"
)

func (b CmdContext) makeCmd() *exec.Cmd {
	return exec.Command(b.command, b.args...)
}

// CmdContext is used to build, customize and execute a command.
// Add more functions to customize the context as needed.
type CmdContext struct {
	command string
	args    []string
	envs    []string
	dir     string
	stdin   io.Reader
	timeout <-chan time.Time
	retries RetriesContext
}

// RetriesContext is used to tweak retry loop.
type RetriesContext struct {
	count    int
	interval time.Duration
}

// WithinDirectory tells Docker the cwd.
func (b *CmdContext) WithinDirectory(path string) *CmdContext {
	b.dir = path
	return b
}

// WithEnvs set envs in context.
func (b *CmdContext) WithEnvs(envs []string) *CmdContext {
	b.envs = envs
	return b
}

// WithTimeout controls maximum duration.
func (b *CmdContext) WithTimeout(t <-chan time.Time) *CmdContext {
	b.timeout = t
	return b
}

// WithRetries sets how many times to retry the command before issuing an error
func (b *CmdContext) WithRetries(count int) *CmdContext {
	b.retries.count = count
	return b
}

// Every interval between 2 retries
func (b *CmdContext) Every(interval time.Duration) *CmdContext {
	b.retries.interval = interval
	return b
}

// WithStdinData feeds via stdin.
func (b CmdContext) WithStdinData(data string) *CmdContext {
	b.stdin = strings.NewReader(data)
	return &b
}

// WithStdinReader feeds via stdin.
func (b CmdContext) WithStdinReader(reader io.Reader) *CmdContext {
	b.stdin = reader
	return &b
}

// ExecOrDie runs a docker command.
func (b CmdContext) ExecOrDie() string {
	str, err := b.Exec()
	log.Debugf("stdout: %s", str)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	return str
}

// Exec runs a docker command.
func (b CmdContext) Exec() (string, error) {
	retry := b.retries.count
	for ; ; retry-- {
		cmd := b.makeCmd()
		cmd.Dir = b.dir
		cmd.Stdin = b.stdin
		if b.envs != nil {
			cmd.Env = b.envs
		}
		stdout, err := Execute(cmd, b.timeout)
		if err == nil || retry < 1 {
			return stdout, err
		}
		time.Sleep(b.retries.interval)
	}
}

//WaitFor waits for a condition to be true
func WaitFor(interval, duration time.Duration, abort <-chan error, condition func() bool) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	timeout := make(chan int)
	go func() {
		time.Sleep(duration)
		close(timeout)
	}()
	for {
		select {
		case err := <-abort:
			return err
		case <-timeout:
			return fmt.Errorf("timeout after %v", duration)
		case <-ticker.C:
			if condition() {
				return nil
			}
		}
	}
}

// Execute executes a command.
// The command cannot be re-used afterwards.
func Execute(cmd *exec.Cmd, timeout <-chan time.Time) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd.Stdout = mergeWriter(cmd.Stdout, &stdout)
	cmd.Stderr = mergeWriter(cmd.Stderr, &stderr)

	log.Infof("Execute '%s %s'", cmd.Path, strings.Join(cmd.Args[1:], " ")) // skip arg[0] as it is printed separately
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("error starting %v:\nCommand stdout:\n%v\nstderr:\n%v\nerror:\n%v", cmd, stdout.String(), stderr.String(), err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- cmd.Wait()
	}()
	select {
	case err := <-errCh:
		if err != nil {
			log.Debugf("%s %s failed: %v", cmd.Path, strings.Join(cmd.Args[1:], " "), err)
			return stderr.String(), fmt.Errorf("error running %v:\nCommand stdout:\n%v\nstderr:\n%v\nerror:\n%v", cmd, stdout.String(), stderr.String(), err)
		}
	case <-timeout:
		log.Debugf("%s %s timed-out", cmd.Path, strings.Join(cmd.Args[1:], " "))
		if err := terminateProcess(cmd); err != nil {
			return "", err
		}
		return "", fmt.Errorf(
			"timed out waiting for command %v:\nCommand stdout:\n%v\nstderr:\n%v",
			cmd.Args, stdout.String(), stderr.String())
	}
	if stderr.String() != "" {
		log.Debugf("stderr: %s", stderr.String())
	}
	return stdout.String(), nil
}

func terminateProcess(cmd *exec.Cmd) error {
	if runtime.GOOS == "windows" {
		return cmd.Process.Kill()
	}
	return cmd.Process.Signal(syscall.SIGTERM)
}

func mergeWriter(other io.Writer, buf io.Writer) io.Writer {
	if other != nil {
		return io.MultiWriter(other, buf)
	}
	return buf
}

// Powershell runs a powershell command.
func Powershell(input string) (string, error) {
	output, err := Execute(exec.Command("powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Unrestricted", "-Command", input), nil)
	if err != nil {
		return "", fmt.Errorf("fail to execute %s: %s", input, err)
	}
	return strings.TrimSpace(output), nil
}
