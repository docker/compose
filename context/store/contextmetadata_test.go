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

package store

import (
	"encoding/json"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
)

type ContextTestSuite struct {
	suite.Suite
}

func (suite *ContextTestSuite) TestDockerContextMetadataKeepAdditionalFields() {
	c := ContextMetadata{
		Description:       "test",
		Type:              "aci",
		StackOrchestrator: "swarm",
		AdditionalFields: map[string]interface{}{
			"foo": "bar",
		},
	}
	jsonBytes, err := json.Marshal(c)
	Expect(err).To(BeNil())
	Expect(string(jsonBytes)).To(Equal(`{"Description":"test","StackOrchestrator":"swarm","Type":"aci","foo":"bar"}`))

	var c2 ContextMetadata
	err = json.Unmarshal(jsonBytes, &c2)
	Expect(err).To(BeNil())
	Expect(c2.AdditionalFields["foo"]).To(Equal("bar"))
	Expect(c2.Type).To(Equal("aci"))
	Expect(c2.StackOrchestrator).To(Equal("swarm"))
	Expect(c2.Description).To(Equal("test"))
}

func TestPs(t *testing.T) {
	RegisterTestingT(t)
	suite.Run(t, new(ContextTestSuite))
}
