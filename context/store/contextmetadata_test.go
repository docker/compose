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
