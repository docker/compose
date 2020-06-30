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

package framework

import (
	"runtime"
	"strings"

	"github.com/robpike/filter"
	"github.com/sirupsen/logrus"
)

func nonEmptyString(s string) bool {
	return strings.TrimSpace(s) != ""
}

// Lines get lines from a raw string
func Lines(output string) []string {
	return filter.Choose(strings.Split(output, "\n"), nonEmptyString).([]string)
}

// Columns get columns from a line
func Columns(line string) []string {
	return filter.Choose(strings.Split(line, " "), nonEmptyString).([]string)
}

// GoldenFile golden file specific to platform
func GoldenFile(name string) string {
	if IsWindows() {
		return name + "-windows.golden"
	}
	return name + ".golden"
}

// IsWindows windows or other GOOS
func IsWindows() bool {
	return runtime.GOOS == "windows"
}

// It runs func
func It(description string, test func()) {
	test()
	logrus.Print("Passed: ", description)
}
