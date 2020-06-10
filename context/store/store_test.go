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
