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

package display

import (
	"github.com/morikuni/aec"
)

type colorFunc func(string) string

var (
	nocolor colorFunc = func(s string) string {
		return s
	}

	// DoneColor is the color function used to render the completion indicator.
	DoneColor    colorFunc = aec.BlueF.Apply
	// TimerColor is the color function used to render elapsed time.
	TimerColor   colorFunc = aec.BlueF.Apply
	// CountColor is the color function used to render counts.
	CountColor   colorFunc = aec.YellowF.Apply
	// WarningColor is the color function used to render warning messages.
	WarningColor colorFunc = aec.YellowF.With(aec.Bold).Apply
	// SuccessColor is the color function used to render success messages.
	SuccessColor colorFunc = aec.GreenF.Apply
	// ErrorColor is the color function used to render error messages.
	ErrorColor   colorFunc = aec.RedF.With(aec.Bold).Apply
	// PrefixColor is the color function used to render service name prefixes.
	PrefixColor  colorFunc = aec.CyanF.Apply
)

// NoColor disables all terminal color output by replacing color functions with pass-through variants.
func NoColor() {
	DoneColor = nocolor
	TimerColor = nocolor
	CountColor = nocolor
	WarningColor = nocolor
	SuccessColor = nocolor
	ErrorColor = nocolor
	PrefixColor = nocolor
}
