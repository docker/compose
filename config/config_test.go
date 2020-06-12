/*
	Copyright (c) 2020 Docker Inc.

	Permission is hereby granted, free of charge, to any person
	obtaining a copy of this software and associated documentation
	files (the "Software"), to deal in the Software without
	restriction, including without limitation the rights to use, copy,
	modify, merge, publish, distribute, sublicense, and/or sell copies
	of the Software, and to permit persons to whom the Software is
	furnished to do so, subject to the following conditions:

	The above copyright notice and this permission notice shall be
	included in all copies or substantial portions of the Software.

	THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
	EXPRESS OR IMPLIED,
	INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
	FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
	IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT
	HOLDERS BE LIABLE FOR ANY CLAIM,
	DAMAGES OR OTHER LIABILITY,
	WHETHER IN AN ACTION OF CONTRACT,
	TORT OR OTHERWISE,
	ARISING FROM, OUT OF OR IN CONNECTION WITH
	THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*/

package config

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

var sampleConfig = []byte(`{
	"otherField": "value",
	"currentContext": "local"
}`)

type ConfigTestSuite struct {
	suite.Suite
	configDir string
}

func (s *ConfigTestSuite) BeforeTest(suite, test string) {
	d, _ := ioutil.TempDir("", "")
	s.configDir = d
}

func (s *ConfigTestSuite) AfterTest(suite, test string) {
	err := os.RemoveAll(s.configDir)
	require.NoError(s.T(), err)
}

func writeSampleConfig(t *testing.T, d string) {
	err := ioutil.WriteFile(filepath.Join(d, ConfigFileName), sampleConfig, 0644)
	require.NoError(t, err)
}

func (s *ConfigTestSuite) TestLoadFile() {
	writeSampleConfig(s.T(), s.configDir)
	f, err := LoadFile(s.configDir)
	require.NoError(s.T(), err)
	require.Equal(s.T(), "local", f.CurrentContext)
}

func (s *ConfigTestSuite) TestOverWriteCurrentContext() {
	writeSampleConfig(s.T(), s.configDir)
	f, err := LoadFile(s.configDir)
	require.NoError(s.T(), err)
	require.Equal(s.T(), "local", f.CurrentContext)

	err = WriteCurrentContext(s.configDir, "overwrite")
	require.NoError(s.T(), err)
	f, err = LoadFile(s.configDir)
	require.NoError(s.T(), err)
	require.Equal(s.T(), "overwrite", f.CurrentContext)

	m := map[string]interface{}{}
	err = loadFile(filepath.Join(s.configDir, ConfigFileName), &m)
	require.NoError(s.T(), err)
	require.Equal(s.T(), "overwrite", m["currentContext"])
	require.Equal(s.T(), "value", m["otherField"])
}

// TestWriteDefaultContextToEmptyConfig tests a specific case seen on the CI:
// panic when setting context to default with empty config file
func (s *ConfigTestSuite) TestWriteDefaultContextToEmptyConfig() {
	err := WriteCurrentContext(s.configDir, "default")
	require.NoError(s.T(), err)
	d, err := ioutil.ReadFile(filepath.Join(s.configDir, ConfigFileName))
	require.NoError(s.T(), err)
	require.Equal(s.T(), string(d), "{}")
}

func TestConfig(t *testing.T) {
	suite.Run(t, new(ConfigTestSuite))
}
