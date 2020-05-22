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
	err := suite.store.Create("test", TypedContext{})
	require.Nil(suite.T(), err)

	err = suite.store.Create("test", TypedContext{})
	require.EqualError(suite.T(), err, `context "test": already exists`)
	require.True(suite.T(), errdefs.IsAlreadyExistsError(err))
}

func (suite *StoreTestSuite) TestGetUnknown() {
	meta, err := suite.store.Get("unknown", nil)
	require.Nil(suite.T(), meta)
	require.EqualError(suite.T(), err, `context "unknown": not found`)
	require.True(suite.T(), errdefs.IsNotFoundError(err))
}

func (suite *StoreTestSuite) TestGet() {
	err := suite.store.Create("test", TypedContext{
		Type:        "type",
		Description: "description",
	})
	require.Nil(suite.T(), err)

	meta, err := suite.store.Get("test", nil)
	require.Nil(suite.T(), err)
	require.NotNil(suite.T(), meta)
	require.Equal(suite.T(), "test", meta.Name)

	require.Equal(suite.T(), "description", meta.Metadata.Description)
	require.Equal(suite.T(), "type", meta.Metadata.Type)
}

func (suite *StoreTestSuite) TestList() {
	err := suite.store.Create("test1", TypedContext{})
	require.Nil(suite.T(), err)

	err = suite.store.Create("test2", TypedContext{})
	require.Nil(suite.T(), err)

	contexts, err := suite.store.List()
	require.Nil(suite.T(), err)

	require.Equal(suite.T(), len(contexts), 3)
	require.Equal(suite.T(), "test1", contexts[0].Name)
	require.Equal(suite.T(), "test2", contexts[1].Name)
	require.Equal(suite.T(), "default", contexts[2].Name)
}

func (suite *StoreTestSuite) TestRemoveNotFound() {
	err := suite.store.Remove("notfound")
	require.EqualError(suite.T(), err, `context "notfound": not found`)
	require.True(suite.T(), errdefs.IsNotFoundError(err))
}

func (suite *StoreTestSuite) TestRemove() {
	err := suite.store.Create("testremove", TypedContext{})
	require.Nil(suite.T(), err)
	contexts, err := suite.store.List()
	require.Nil(suite.T(), err)
	require.Equal(suite.T(), len(contexts), 2)

	err = suite.store.Remove("testremove")
	require.Nil(suite.T(), err)
	contexts, err = suite.store.List()
	require.Nil(suite.T(), err)
	// The default context is always here, that's why we
	// have len(contexts) == 1
	require.Equal(suite.T(), len(contexts), 1)
}

func TestExampleTestSuite(t *testing.T) {
	suite.Run(t, new(StoreTestSuite))
}
