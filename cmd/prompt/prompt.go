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
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/docker/cli/cli/streams"

	"github.com/docker/compose/v5/pkg/utils"
)

//go:generate mockgen -destination=./prompt_mock.go -self_package "github.com/docker/compose/v5/pkg/prompt" -package=prompt . UI

var errInterrupt = errors.New("interrupt")

// UI - prompt user input
type UI interface {
	Confirm(message string, defaultValue bool) (bool, error)
}

func NewPrompt(stdin *streams.In, stdout *streams.Out) UI {
	if stdin.IsTerminal() {
		return User{stdin: stdin, reader: bufio.NewReader(stdin), stdout: stdout}
	}
	return Pipe{stdin: stdin, stdout: stdout}
}

// User - in a terminal
type User struct {
	stdout io.Writer
	stdin  *streams.In
	reader *bufio.Reader
}

// Confirm asks for yes or no input
func (u User) Confirm(message string, defaultValue bool) (bool, error) {
	if err := u.stdin.SetRawTerminal(); err != nil {
		return false, err
	}
	defer u.stdin.RestoreTerminal()

	prompt := " [y/N]: "
	if defaultValue {
		prompt = " [Y/n]: "
	}

	for {
		_, _ = fmt.Fprint(u.stdout, message+prompt)

		answer, err := readLine(u.reader, u.stdout)
		if err != nil {
			return false, err
		}

		switch strings.ToLower(strings.TrimSpace(answer)) {
		case "":
			return defaultValue, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		}
	}
}

func readLine(in io.RuneReader, out io.Writer) (string, error) {
	var line []rune

	for {
		ch, _, err := in.ReadRune()
		if err != nil {
			return "", err
		}

		switch ch {
		case 3: // Ctrl+C
			_, _ = fmt.Fprint(out, "\r\n")
			return "", errInterrupt

		case 4: // Ctrl+D
			return "", io.EOF

		case '\r', '\n':
			_, _ = fmt.Fprint(out, "\r\n")
			return string(line), nil

		case 127: // Backspace
			if len(line) > 0 {
				line = line[:len(line)-1]
				_, _ = fmt.Fprint(out, "\b \b")
			}

		default:
			if unicode.IsControl(ch) {
				continue
			}
			line = append(line, ch)
			_, _ = fmt.Fprintf(out, "%c", ch)
		}
	}
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
	_, _ = fmt.Fscanln(u.stdin, &answer)
	return utils.StringToBool(answer), nil
}
