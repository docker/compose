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
