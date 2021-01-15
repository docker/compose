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

package framework

import (
	"context"
	"io/ioutil"
	"os"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"

	apicontext "github.com/docker/compose-cli/api/context"
	"github.com/docker/compose-cli/api/context/store"
)

// TestCLI is a helper struct for CLI tests.
type TestCLI struct {
	ctx    context.Context
	writer *os.File
	reader *os.File
}

// NewTestCLI returns a CLI testing helper.
func NewTestCLI(t *testing.T) *TestCLI {
	dir, err := ioutil.TempDir("", "store")
	assert.Check(t, cmp.Nil(err))

	originalStdout := os.Stdout

	t.Cleanup(func() {
		os.Stdout = originalStdout
		_ = os.RemoveAll(dir)
	})

	s, err := store.New(dir)
	assert.Check(t, cmp.Nil(err))
	err = s.Create("example", "example", "", store.ContextMetadata{})
	assert.Check(t, cmp.Nil(err))

	ctx := context.Background()
	ctx = store.WithContextStore(ctx, s)
	ctx = apicontext.WithCurrentContext(ctx, "example")

	r, w, err := os.Pipe()
	os.Stdout = w
	assert.Check(t, cmp.Nil(err))
	return &TestCLI{ctx, w, r}
}

// Context returns a configured context
func (c *TestCLI) Context() context.Context {
	return c.ctx
}

// GetStdOut returns the output of the command
func (c *TestCLI) GetStdOut() string {
	_ = c.writer.Close()
	out, _ := ioutil.ReadAll(c.reader)
	return string(out)
}
