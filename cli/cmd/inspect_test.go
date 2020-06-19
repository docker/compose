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

package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gotest.tools/v3/golden"

	_ "github.com/docker/api/example"
	"github.com/docker/api/tests/framework"
)

type InspectSuite struct {
	framework.CliSuite
}

func (sut *InspectSuite) TestInspectId() {
	err := runInspect(sut.Context(), "id")
	require.Nil(sut.T(), err)
	golden.Assert(sut.T(), sut.GetStdOut(), "inspect-out-id.golden")
}

func TestInspect(t *testing.T) {
	suite.Run(t, new(InspectSuite))
}
