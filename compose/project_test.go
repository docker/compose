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

package compose

import (
	"os"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
)

type ComposeTest struct {
	suite.Suite
}

func (suite *ComposeTest) TestParseComposeFile() {
	files := []string{"../tests/composefiles/aci-demo/aci_demo_port.yaml"}
	config, err := parseConfigs(files)
	Expect(err).To(BeNil())
	services := config[0].Config["services"].(map[string]interface{})
	Expect(len(services)).To(Equal(3))
}

func (suite *ComposeTest) TestParseComposeStdin() {
	files := []string{"-"}
	f, err := os.Open("../tests/composefiles/aci-demo/aci_demo_port.yaml")
	Expect(err).To(BeNil())
	defer func() {
		err := f.Close()
		Expect(err).To(BeNil())
	}()
	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }() // Restore original Stdin

	os.Stdin = f
	config, err := parseConfigs(files)
	Expect(err).To(BeNil())
	services := config[0].Config["services"].(map[string]interface{})
	Expect(len(services)).To(Equal(3))
}

func TestComposeProject(t *testing.T) {
	RegisterTestingT(t)
	suite.Run(t, new(ComposeTest))
}
