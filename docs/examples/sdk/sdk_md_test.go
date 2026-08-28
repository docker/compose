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

package main

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

// the compiler keeps the example alive; the doc keeps it honest
var _ = customService

var fencedGoBlocks = regexp.MustCompile("(?s)```go\n(.*?)```")

// The fenced Go blocks in docs/sdk.md are real, compiled code: the first is
// main.go from the package clause down, the second is customService's body
// in options.go. The compiler catches API drift; this test catches wording
// drift between the documentation and the compiled examples.
func TestSDKDocExamplesMatchCompiledCode(t *testing.T) {
	md, err := os.ReadFile("../../sdk.md")
	assert.NilError(t, err)

	blocks := fencedGoBlocks.FindAllStringSubmatch(string(md), -1)
	assert.Equal(t, len(blocks), 2, "docs/sdk.md is expected to hold exactly two fenced Go blocks")

	mainGo, err := os.ReadFile("main.go")
	assert.NilError(t, err)
	// the example file carries a license header and this explanatory comment
	// above the package clause; the doc block starts at the package clause
	_, code, found := strings.Cut(string(mainGo), "package main")
	assert.Assert(t, found)
	assert.Equal(t, blocks[0][1], "package main"+code,
		"the first Go block of docs/sdk.md must match docs/examples/sdk/main.go from its package clause down")

	optionsGo, err := os.ReadFile("options.go")
	assert.NilError(t, err)
	assert.Assert(t, strings.Contains(string(optionsGo), strings.TrimRight(blocks[1][1], "\n")),
		"the second Go block of docs/sdk.md must appear verbatim in docs/examples/sdk/options.go")
}
