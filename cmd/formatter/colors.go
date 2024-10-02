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

package formatter

import (
	"fmt"
	"strconv"
	"sync"

	"github.com/docker/cli/cli/command"
)

var names = []string{
	"grey",
	"red",
	"green",
	"yellow",
	"blue",
	"magenta",
	"cyan",
	"white",
}

const (
	BOLD      = "1"
	FAINT     = "2"
	ITALIC    = "3"
	UNDERLINE = "4"
)

const (
	RESET = "0"
	CYAN  = "36"
)

const (
	// Never use ANSI codes
	Never = "never"

	// Always use ANSI codes
	Always = "always"

	// Auto detect terminal is a tty and can use ANSI codes
	Auto = "auto"
)

// SetANSIMode configure formatter for colored output on ANSI-compliant console
func SetANSIMode(streams command.Streams, ansi string) {
	if !useAnsi(streams, ansi) {
		nextColor = func() colorFunc {
			return monochrome
		}
		disableAnsi = true
	}
}

func useAnsi(streams command.Streams, ansi string) bool {
	switch ansi {
	case Always:
		return true
	case Auto:
		return streams.Out().IsTerminal()
	}
	return false
}

// colorFunc use ANSI codes to render colored text on console
type colorFunc func(s string) string

var monochrome = func(s string) string {
	return s
}

func ansiColor(code, s string, formatOpts ...string) string {
	return fmt.Sprintf("%s%s%s", ansiColorCode(code, formatOpts...), s, ansiColorCode("0"))
}

// Everything about ansiColorCode color https://hyperskill.org/learn/step/18193
func ansiColorCode(code string, formatOpts ...string) string {
	res := "\033["
	for _, c := range formatOpts {
		res = fmt.Sprintf("%s%s;", res, c)
	}
	return fmt.Sprintf("%s%sm", res, code)
}

func makeColorFunc(code string) colorFunc {
	return func(s string) string {
		return ansiColor(code, s)
	}
}

var nextColor = rainbowColor
var rainbow []colorFunc
var currentIndex = 0
var mutex sync.Mutex

func rainbowColor() colorFunc {
	mutex.Lock()
	defer mutex.Unlock()
	result := rainbow[currentIndex]
	currentIndex = (currentIndex + 1) % len(rainbow)
	return result
}

func init() {
	colors := map[string]colorFunc{}
	for i, name := range names {
		colors[name] = makeColorFunc(strconv.Itoa(30 + i))
		colors["intense_"+name] = makeColorFunc(strconv.Itoa(30+i) + ";1")
	}
	rainbow = []colorFunc{
		colors["cyan"],
		colors["yellow"],
		colors["green"],
		colors["magenta"],
		colors["blue"],
		colors["intense_cyan"],
		colors["intense_yellow"],
		colors["intense_green"],
		colors["intense_magenta"],
		colors["intense_blue"],
	}
}
