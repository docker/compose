/*
   Copyright 2020 Docker, Inc.

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

package ecs

import (
	"fmt"
	"strconv"
)

var NAMES = []string{
	"grey",
	"red",
	"green",
	"yellow",
	"blue",
	"magenta",
	"cyan",
	"white",
}

var COLORS map[string]ColorFunc

// ColorFunc use ANSI codes to render colored text on console
type ColorFunc func(s string) string

var Monochrome = func(s string) string {
	return s
}

func ansiColor(code, s string) string {
	return fmt.Sprintf("%s%s%s", ansi(code), s, ansi("0"))
}

func ansi(code string) string {
	return fmt.Sprintf("\033[%sm", code)
}

func makeColorFunc(code string) ColorFunc {
	return func(s string) string {
		return ansiColor(code, s)
	}
}

var Rainbow = make(chan ColorFunc)

func init() {
	COLORS = map[string]ColorFunc{}
	for i, name := range NAMES {
		COLORS[name] = makeColorFunc(strconv.Itoa(30 + i))
		COLORS["intense_"+name] = makeColorFunc(strconv.Itoa(30+i) + ";1")
	}

	go func() {
		i := 0
		rainbow := []ColorFunc{
			COLORS["cyan"],
			COLORS["yellow"],
			COLORS["green"],
			COLORS["magenta"],
			COLORS["blue"],
			COLORS["intense_cyan"],
			COLORS["intense_yellow"],
			COLORS["intense_green"],
			COLORS["intense_magenta"],
			COLORS["intense_blue"],
		}

		for {
			Rainbow <- rainbow[i]
			i = (i + 1) % len(rainbow)
		}
	}()
}
