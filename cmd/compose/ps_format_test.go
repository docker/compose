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
	"testing"

	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/cli/cli/streams"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/mocks"
)

func TestPsCommandDefaultFormat(t *testing.T) {
	// Test that the format flag has empty string as default
	projectOpts := &ProjectOptions{}
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	
	cli := mocks.NewMockCli(mockCtrl)
	cli.EXPECT().ConfigFile().Return(configfile.New("test")).AnyTimes()
	cli.EXPECT().Out().Return(&streams.Out{}).AnyTimes()
	cli.EXPECT().Err().Return(&streams.Out{}).AnyTimes()
	
	backendOptions := &BackendOptions{}
	cmd := psCommand(projectOpts, cli, backendOptions)

	// Check default value of format flag
	formatFlag := cmd.Flags().Lookup("format")
	assert.Equal(t, formatFlag.DefValue, "")
}
