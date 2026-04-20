/*
   Copyright 2024 Docker Compose CLI authors

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

package formatter

import (
	"fmt"

	"github.com/acarl005/stripansi"
	"github.com/morikuni/aec"
)

var disableAnsi bool

func saveCursor() {
	if disableAnsi {
		return
	}
	// see https://github.com/morikuni/aec/pull/5
	fmt.Print(aec.Save)
}

func restoreCursor() {
	if disableAnsi {
		return
	}
	// see https://github.com/morikuni/aec/pull/5
	fmt.Print(aec.Restore)
}

func showCursor() {
	if disableAnsi {
		return
	}
	fmt.Print(aec.Show)
}

func moveCursor(y, x int) {
	if disableAnsi {
		return
	}
	fmt.Print(aec.Position(uint(y), uint(x)))
}

func carriageReturn() {
	if disableAnsi {
		return
	}
	fmt.Print(aec.Column(0))
}

func clearLine() {
	if disableAnsi {
		return
	}
	// Does not move cursor from its current position
	fmt.Print(aec.EraseLine(aec.EraseModes.Tail))
}

func moveCursorUp(lines int) {
	if disableAnsi {
		return
	}
	// Does not add new lines
	fmt.Print(aec.Up(uint(lines)))
}

func moveCursorDown(lines int) {
	if disableAnsi {
		return
	}
	// Does not add new lines
	fmt.Print(aec.Down(uint(lines)))
}

func newLine() {
	fmt.Print("\012")
}

// lenAnsi returns the visible length of s after stripping ANSI escape codes.
func lenAnsi(s string) int {
	return len(stripansi.Strip(s))
}

// OSC8Link wraps text in an OSC 8 terminal hyperlink escape sequence with
// underline styling, making it clickable in supported terminal emulators.
// When ANSI output is disabled, returns the plain text without escape sequences.
func OSC8Link(url, text string) string {
	if disableAnsi {
		return text
	}
	return "\033]8;;" + url + "\033\\\033[4m" + text + "\033[24m\033]8;;\033\\"
}
