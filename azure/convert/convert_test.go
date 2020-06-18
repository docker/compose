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
