/*
   Copyright 2025 Docker Compose CLI authors

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
	"bytes"
	"testing"

	"github.com/docker/cli/cli/streams"
	"github.com/docker/compose/v2/internal"
	"github.com/docker/compose/v2/pkg/mocks"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"
)

func TestVersionCommand(t *testing.T) {
	originalVersion := internal.Version
	defer func() {
		internal.Version = originalVersion
	}()
	internal.Version = "v9.9.9-test"

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "default",
			args: []string{},
			want: "Docker Compose version v9.9.9-test\n",
		},
		{
			name: "short flag",
			args: []string{"--short"},
			want: "9.9.9-test\n",
		},
		{
			name: "json flag",
			args: []string{"--format", "json"},
			want: `{"version":"v9.9.9-test"}` + "\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			buf := new(bytes.Buffer)
			cli := mocks.NewMockCli(ctrl)
			cli.EXPECT().Out().Return(streams.NewOut(buf)).AnyTimes()

			cmd := versionCommand(cli)
			cmd.SetArgs(test.args)
			err := cmd.Execute()
			assert.NilError(t, err)

			assert.Equal(t, test.want, buf.String())
		})
	}
}
