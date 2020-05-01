package convert

import (
	"testing"

	"github.com/docker/api/compose"
	"github.com/docker/api/context/store"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	projectName         = "TEST"
	expectedProjectName = "test"
)

type ConvertTestSuite struct {
	suite.Suite
	ctx     store.AciContext
	project compose.Project
}

func (suite *ConvertTestSuite) BeforeTest(suiteName, testName string) {
	ctx := store.AciContext{
		SubscriptionID: "subID",
		ResourceGroup:  "rg",
		Location:       "eu",
	}
	project := compose.Project{
		Name: projectName,
	}

	suite.ctx = ctx
	suite.project = project
}

func (suite *ConvertTestSuite) TestProjectName() {
	containerGroup, err := ToContainerGroup(suite.ctx, suite.project)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), *containerGroup.Name, expectedProjectName)
}

func TestConvertTestSuite(t *testing.T) {
	suite.Run(t, new(ConvertTestSuite))
}
