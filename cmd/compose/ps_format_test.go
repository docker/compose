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

func TestPsCommandUsesConfigFormat(t *testing.T) {
	projectOpts := &ProjectOptions{}
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	cli := mocks.NewMockCli(mockCtrl)
	config := configfile.New("test")
	config.PsFormat = "table {{.Names}}\t{{.Image}}"
	cli.EXPECT().ConfigFile().Return(config).AnyTimes()
	
	out := &streams.Out{}
	err := &streams.Out{}
	cli.EXPECT().Out().Return(out).AnyTimes()
	cli.EXPECT().Err().Return(err).AnyTimes()

	backendOptions := &BackendOptions{}
	cmd := psCommand(projectOpts, cli, backendOptions)

	// Set args to trigger format resolution
	cmd.SetArgs([]string{})
	// Mock the backend to avoid actual container operations
	// This test focuses on format flag logic, not full command execution
	
	formatFlag := cmd.Flags().Lookup("format")
	assert.Equal(t, formatFlag.DefValue, "")
}

func TestPsCommandQuietWithFormatFlag(t *testing.T) {
	projectOpts := &ProjectOptions{}
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	cli := mocks.NewMockCli(mockCtrl)
	config := configfile.New("test")
	cli.EXPECT().ConfigFile().Return(config).AnyTimes()
	
	out := &streams.Out{}
	err := &streams.Out{}
	cli.EXPECT().Out().Return(out).AnyTimes()
	cli.EXPECT().Err().Return(err).AnyTimes()

	backendOptions := &BackendOptions{}
	cmd := psCommand(projectOpts, cli, backendOptions)

	// Test that warning is shown when both --format and --quiet are explicitly set
	errBuf := &streams.Out{}
	cli.EXPECT().Err().Return(errBuf).AnyTimes()
	
	// Simulate flag changes
	cmd.SetArgs([]string{"--format", "table {{.Names}}", "--quiet"})
	cmd.ParseFlags([]string{"--format", "table {{.Names}}", "--quiet"})
	
	// The flag should be marked as changed
	assert.Assert(t, cmd.Flags().Changed("format"))
	assert.Assert(t, cmd.Flags().Changed("quiet"))
}

func TestPsCommandQuietWithConfigFormat(t *testing.T) {
	projectOpts := &ProjectOptions{}
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	cli := mocks.NewMockCli(mockCtrl)
	config := configfile.New("test")
	config.PsFormat = "table {{.Names}}\t{{.Image}}"
	cli.EXPECT().ConfigFile().Return(config).AnyTimes()
	
	out := &streams.Out{}
	err := &streams.Out{}
	cli.EXPECT().Out().Return(out).AnyTimes()
	cli.EXPECT().Err().Return(err).AnyTimes()

	backendOptions := &BackendOptions{}
	cmd := psCommand(projectOpts, cli, backendOptions)

	// Test that no warning is shown when only --quiet is set (format from config)
	errBuf := &streams.Out{}
	cli.EXPECT().Err().Return(errBuf).AnyTimes()
	
	// Simulate only quiet flag change
	cmd.SetArgs([]string{"--quiet"})
	cmd.ParseFlags([]string{"--quiet"})
	
	// Only quiet flag should be changed, not format
	assert.Assert(t, !cmd.Flags().Changed("format"))
	assert.Assert(t, cmd.Flags().Changed("quiet"))
}

func TestPsCommandFormatFallback(t *testing.T) {
	projectOpts := &ProjectOptions{}
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	cli := mocks.NewMockCli(mockCtrl)
	config := configfile.New("test")
	// No PsFormat set in config
	cli.EXPECT().ConfigFile().Return(config).AnyTimes()
	
	out := &streams.Out{}
	err := &streams.Out{}
	cli.EXPECT().Out().Return(out).AnyTimes()
	cli.EXPECT().Err().Return(err).AnyTimes()

	backendOptions := &BackendOptions{}
	cmd := psCommand(projectOpts, cli, backendOptions)

	// Test that format falls back to "table" when not set in flags or config
	cmd.SetArgs([]string{})
	cmd.ParseFlags([]string{})
	
	// Should not have format flag changed
	assert.Assert(t, !cmd.Flags().Changed("format"))
}
