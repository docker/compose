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

package compose

import (
	"os"
	"testing"

	"github.com/docker/cli/cli/streams"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/mocks"
)

func TestExecInteractiveDefaultOffWhenStdoutNotTTY(t *testing.T) {
	r, w, err := os.Pipe()
	assert.NilError(t, err)
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})

	cli := mocks.NewMockCli(gomock.NewController(t))
	cli.EXPECT().Out().Return(streams.NewOut(w)).AnyTimes()

	cmd := execCommand(&ProjectOptions{}, cli, &BackendOptions{})
	assert.Equal(t, cmd.Flags().Lookup("interactive").DefValue, "false")
	assert.Equal(t, cmd.Flags().Lookup("no-tty").DefValue, "true")
}
