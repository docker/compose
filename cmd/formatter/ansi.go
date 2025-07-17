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
)

var disableAnsi bool

func ansi(code string) string {
	return fmt.Sprintf("\033%s", code)
}

func saveCursor() {
	if disableAnsi {
		return
	}
	fmt.Print(ansi("7"))
}

func restoreCursor() {
	if disableAnsi {
		return
	}
	fmt.Print(ansi("8"))
}

func hideCursor() {
	if disableAnsi {
		return
	}
	fmt.Print(ansi("[?25l"))
}

func showCursor() {
	if disableAnsi {
		return
	}
	fmt.Print(ansi("[?25h"))
}

func moveCursor(y, x int) {
	if disableAnsi {
		return
	}
	fmt.Print(ansi(fmt.Sprintf("[%d;%dH", y, x)))
}

func carriageReturn() {
	if disableAnsi {
		return
	}
	fmt.Print(ansi(fmt.Sprintf("[%dG", 0)))
}

func clearLine() {
	if disableAnsi {
		return
	}
	// Does not move cursor from its current position
	fmt.Print(ansi("[2K"))
}

func moveCursorDown(lines int) {
	if disableAnsi {
		return
	}
	// Does not add new lines
	fmt.Print(ansi(fmt.Sprintf("[%dB", lines)))
}
