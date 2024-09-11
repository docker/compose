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
	"fmt"
	"io"

	"github.com/AlecAivazis/survey/v2"
	"github.com/docker/cli/cli/streams"
	"github.com/docker/compose/v2/pkg/utils"
)

//go:generate mockgen -destination=./prompt_mock.go -self_package "github.com/docker/compose/v2/pkg/prompt" -package=prompt . UI

// UI - prompt user input
type UI interface {
	Confirm(message string, defaultValue bool) (bool, error)
}

func NewPrompt(stdin *streams.In, stdout *streams.Out) UI {
	if stdin.IsTerminal() {
		return User{stdin: streamsFileReader{stdin}, stdout: streamsFileWriter{stdout}}
	}
	return Pipe{stdin: stdin, stdout: stdout}
}

// User - in a terminal
type User struct {
	stdout streamsFileWriter
	stdin  streamsFileReader
}

// adapt streams.Out to terminal.FileWriter
type streamsFileWriter struct {
	stream *streams.Out
}

func (s streamsFileWriter) Write(p []byte) (n int, err error) {
	return s.stream.Write(p)
}

func (s streamsFileWriter) Fd() uintptr {
	return s.stream.FD()
}

// adapt streams.In to terminal.FileReader
type streamsFileReader struct {
	stream *streams.In
}

func (s streamsFileReader) Read(p []byte) (n int, err error) {
	return s.stream.Read(p)
}

func (s streamsFileReader) Fd() uintptr {
	return s.stream.FD()
}

// Confirm asks for yes or no input
func (u User) Confirm(message string, defaultValue bool) (bool, error) {
	qs := &survey.Confirm{
		Message: message,
		Default: defaultValue,
	}
	var b bool
	err := survey.AskOne(qs, &b, func(options *survey.AskOptions) error {
		options.Stdio.In = u.stdin
		options.Stdio.Out = u.stdout
		return nil
	})
	return b, err
}

// Pipe - aggregates prompt methods
type Pipe struct {
	stdout io.Writer
	stdin  io.Reader
}

// Confirm asks for yes or no input
func (u Pipe) Confirm(message string, defaultValue bool) (bool, error) {
	_, _ = fmt.Fprint(u.stdout, message)
	var answer string
	_, _ = fmt.Scanln(&answer)
	return utils.StringToBool(answer), nil
}
