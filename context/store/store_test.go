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
	_ "crypto/sha256"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/docker/api/errdefs"
)

type StoreTestSuite struct {
	suite.Suite
	store Store
	dir   string
}

func (suite *StoreTestSuite) BeforeTest(suiteName, testName string) {
	dir, err := ioutil.TempDir("", "store")
	require.Nil(suite.T(), err)

	store, err := New(WithRoot(dir))
	require.Nil(suite.T(), err)

	suite.dir = dir
	suite.store = store
}

func (suite *StoreTestSuite) AfterTest(suiteName, testName string) {
	err := os.RemoveAll(suite.dir)
	require.Nil(suite.T(), err)
}

func (suite *StoreTestSuite) TestCreate() {
	err := suite.store.Create("test", "test", "description", ContextMetadata{})
	require.Nil(suite.T(), err)

	err = suite.store.Create("test", "test", "descrsiption", ContextMetadata{})
	require.EqualError(suite.T(), err, `context "test": already exists`)
	require.True(suite.T(), errdefs.IsAlreadyExistsError(err))
}

func (suite *StoreTestSuite) TestGetEndpoint() {
	err := suite.store.Create("aci", "aci", "description", AciContext{
		Location: "eu",
	})
	require.Nil(suite.T(), err)

	var ctx AciContext
	err = suite.store.GetEndpoint("aci", &ctx)
	assert.Equal(suite.T(), nil, err)
	assert.Equal(suite.T(), "eu", ctx.Location)

	var exampleCtx ExampleContext
	err = suite.store.GetEndpoint("aci", &exampleCtx)
	assert.EqualError(suite.T(), err, "wrong context type")
}

func (suite *StoreTestSuite) TestGetUnknown() {
	meta, err := suite.store.Get("unknown")
	require.Nil(suite.T(), meta)
	require.EqualError(suite.T(), err, `context "unknown": not found`)
	require.True(suite.T(), errdefs.IsNotFoundError(err))
}

func (suite *StoreTestSuite) TestGet() {
	err := suite.store.Create("test", "type", "description", ContextMetadata{})
	require.Nil(suite.T(), err)

	meta, err := suite.store.Get("test")
	require.Nil(suite.T(), err)
	require.NotNil(suite.T(), meta)
	require.Equal(suite.T(), "test", meta.Name)

	require.Equal(suite.T(), "description", meta.Metadata.Description)
	require.Equal(suite.T(), "type", meta.Type())
}

func (suite *StoreTestSuite) TestRemoveNotFound() {
	err := suite.store.Remove("notfound")
	require.EqualError(suite.T(), err, `context "notfound": not found`)
	require.True(suite.T(), errdefs.IsNotFoundError(err))
}

func (suite *StoreTestSuite) TestRemove() {
	err := suite.store.Create("testremove", "type", "description", ContextMetadata{})
	require.Nil(suite.T(), err)

	meta, err := suite.store.Get("testremove")
	require.Nil(suite.T(), err)
	require.NotNil(suite.T(), meta)

	err = suite.store.Remove("testremove")
	require.Nil(suite.T(), err)

	meta, err = suite.store.Get("testremove")
	require.EqualError(suite.T(), err, `context "testremove": not found`)
	require.Nil(suite.T(), meta)

}

func TestExampleTestSuite(t *testing.T) {
	suite.Run(t, new(StoreTestSuite))
}
