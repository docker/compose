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
