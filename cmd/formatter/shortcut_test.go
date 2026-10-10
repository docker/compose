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
	"context"
	"os"
	"testing"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/eiannone/keyboard"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
)

func TestRunHandlesKeysUntilContextIsDone(t *testing.T) {
	detached := make(chan struct{})
	keys := make(chan keyboard.KeyEvent, 1)
	lk := NewKeyboardManager(false, false, make(chan os.Signal, 1))
	lk.keys = keys
	lk.EnableDetach(func() { close(detached) })

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		lk.Run(ctx, &types.Project{}, api.UpOptions{})
		close(done)
	}()

	keys <- keyboard.KeyEvent{Rune: 'd'}
	select {
	case <-detached:
	case <-time.After(5 * time.Second):
		t.Fatal("the detach key was not handled")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run must return once its context is done")
	}
	assert.NilError(t, lk.Close(), "Close must be harmless when the keyboard was never opened")
	assert.NilError(t, lk.Close(), "Close must be idempotent")
}

func TestRunReturnsWhenKeyboardIsClosed(t *testing.T) {
	keys := make(chan keyboard.KeyEvent)
	lk := NewKeyboardManager(false, false, make(chan os.Signal, 1))
	lk.keys = keys

	done := make(chan struct{})
	go func() {
		lk.Run(t.Context(), &types.Project{}, api.UpOptions{})
		close(done)
	}()

	close(keys) // what keyboard.Close does to the channel it handed out
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run must return once the keyboard channel is closed")
	}
}

func TestRunWithoutOpenReturnsImmediately(t *testing.T) {
	lk := NewKeyboardManager(false, false, make(chan os.Signal, 1))
	done := make(chan struct{})
	go func() {
		lk.Run(t.Context(), &types.Project{}, api.UpOptions{})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run must not wait for keys that can't come")
	}
}
