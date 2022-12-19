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

package progress

import (
	"context"
	"os"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

func TestNoopWriter(t *testing.T) {
	todo := context.TODO()
	writer := ContextWriter(todo)

	assert.Equal(t, writer, &noopWriter{})
}

func TestRunWithStatusWithoutCustomContextWriter(t *testing.T) {
	r, w, err := os.Pipe()
	assert.NilError(t, err)

	os.Stderr = w // mock Stderr for default writer just for testing purpose

	result := make(chan string)
	go func() {
		buf := make([]byte, 256)
		n, _ := r.Read(buf)
		result <- string(buf[:n])
	}()

	// run without any custom writer, so it will use the default writer
	_, err = RunWithStatus(context.TODO(), func(ctx context.Context) (string, error) {
		ContextWriter(ctx).Event(Event{Text: "pass"})
		return "test", nil
	})

	assert.NilError(t, err)

	actual := <-result
	assert.Equal(t, strings.TrimSpace(actual), "pass")
}

func TestRunWithStatusrWithCustomContextWriter(t *testing.T) {
	r, w, err := os.Pipe()
	assert.NilError(t, err)

	writer, err := NewWriter(w) // custom writer
	assert.NilError(t, err)

	result := make(chan string)
	go func() {
		buf := make([]byte, 256)
		n, _ := r.Read(buf)
		result <- string(buf[:n])
	}()

	// attach the custom writer to the context
	ctx := WithContextWriter(context.TODO(), writer)

	// run with the custom writer
	_, err = RunWithStatus(ctx, func(ctx context.Context) (string, error) {
		ContextWriter(ctx).Event(Event{Text: "pass"})
		return "test", nil
	})

	assert.NilError(t, err)

	actual := <-result
	assert.Equal(t, strings.TrimSpace(actual), "pass")
}
