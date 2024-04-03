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
)

var disableAnsi bool

func ansi(code string) string {
	return fmt.Sprintf("\033%s", code)
}
func SaveCursor() {
	if disableAnsi {
		return
	}
	fmt.Print(ansi("7"))
}
func RestoreCursor() {
	if disableAnsi {
		return
	}
	fmt.Print(ansi("8"))
}
func HideCursor() {
	if disableAnsi {
		return
	}
	fmt.Print(ansi("[?25l"))
}
func ShowCursor() {
	if disableAnsi {
		return
	}
	fmt.Print(ansi("[?25h"))
}
func MoveCursor(y, x int) {
	if disableAnsi {
		return
	}
	fmt.Print(ansi(fmt.Sprintf("[%d;%dH", y, x)))
}
func MoveCursorX(pos int) {
	if disableAnsi {
		return
	}
	fmt.Print(ansi(fmt.Sprintf("[%dG", pos)))
}
func ClearLine() {
	if disableAnsi {
		return
	}
	// Does not move cursor from its current position
	fmt.Print(ansi("[2K"))
}
func MoveCursorUp(lines int) {
	if disableAnsi {
		return
	}
	// Does not add new lines
	fmt.Print(ansi(fmt.Sprintf("[%dA", lines)))
}
func MoveCursorDown(lines int) {
	if disableAnsi {
		return
	}
	// Does not add new lines
	fmt.Print(ansi(fmt.Sprintf("[%dB", lines)))
}
func NewLine() {
	// Like \n
	fmt.Print("\012")
}
func lenAnsi(s string) int {
	// len has into consideration ansi codes, if we want
	// the len of the actual len(string) we need to strip
	// all ansi codes
	return len(stripansi.Strip(s))
}
