package e2e

import (
	"strings"
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
	m.NewDockerCommand("context", "rm", "test-context").ExecOrDie()
	m.NewDockerCommand("context", "use", "default").ExecOrDie()
}

func (m *LocalBackendTestSuite) TestPs() {
	out := m.NewDockerCommand("ps").ExecOrDie()
	require.Equal(m.T(), "CONTAINER ID        IMAGE               COMMAND             STATUS              PORTS\n", out)
}

func (m *LocalBackendTestSuite) TestRun() {
	_, err := m.NewDockerCommand("run", "--name", "nginx", "nginx").Exec()
	require.Nil(m.T(), err)
	out := m.NewDockerCommand("ps").ExecOrDie()
	defer func() {
		m.NewDockerCommand("rm", "-f", "nginx").ExecOrDie()
	}()
	lines := strings.Split(out, "\n")
	assert.Equal(m.T(), 3, len(lines))
}

func (m *LocalBackendTestSuite) TestRunWithPorts() {
	_, err := m.NewDockerCommand("run", "--name", "nginx", "-p", "8080:80", "nginx").Exec()
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
