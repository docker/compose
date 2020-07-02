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

package e2e

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/docker/api/tests/framework"
)

type LocalBackendTestSuite struct {
	framework.Suite
}

func (m *LocalBackendTestSuite) BeforeTest(suiteName string, testName string) {
	m.NewDockerCommand("context", "create", "local", "test-context").ExecOrDie()
	m.NewDockerCommand("context", "use", "test-context").ExecOrDie()
}

func (m *LocalBackendTestSuite) AfterTest(suiteName string, testName string) {
	m.NewDockerCommand("context", "rm", "-f", "test-context").ExecOrDie()
}

func (m *LocalBackendTestSuite) TestPs() {
	out := m.NewDockerCommand("ps").ExecOrDie()
	require.Equal(m.T(), "CONTAINER ID        IMAGE               COMMAND             STATUS              PORTS\n", out)
}

func (m *LocalBackendTestSuite) TestRun() {
	_, err := m.NewDockerCommand("run", "-d", "--name", "nginx", "nginx").Exec()
	require.Nil(m.T(), err)
	out := m.NewDockerCommand("ps").ExecOrDie()
	defer func() {
		m.NewDockerCommand("rm", "-f", "nginx").ExecOrDie()
	}()
	assert.Contains(m.T(), out, "nginx")
}

func (m *LocalBackendTestSuite) TestRunWithPorts() {
	_, err := m.NewDockerCommand("run", "-d", "--name", "nginx", "-p", "8080:80", "nginx").Exec()
	require.Nil(m.T(), err)
	out := m.NewDockerCommand("ps").ExecOrDie()
	defer func() {
		m.NewDockerCommand("rm", "-f", "nginx").ExecOrDie()
	}()
	assert.Contains(m.T(), out, "8080")

	out = m.NewDockerCommand("inspect", "nginx").ExecOrDie()
	assert.Contains(m.T(), out, "\"Status\": \"running\"")
}

func (m *LocalBackendTestSuite) TestInspectNotFound() {
	out, _ := m.NewDockerCommand("inspect", "nonexistentcontainer").Exec()
	assert.Contains(m.T(), out, "Error: No such container: nonexistentcontainer")
}

func TestLocalBackendTestSuite(t *testing.T) {
	suite.Run(t, new(LocalBackendTestSuite))
}
