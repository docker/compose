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
)

type StoreTestSuite struct {
	suite.Suite
	store Store
	dir   string
}

func (suite *StoreTestSuite) BeforeTest(suiteName, testName string) {
	dir, err := ioutil.TempDir("", "store")
	require.Nil(suite.T(), err)

	store, err := New(dir)
	require.Nil(suite.T(), err)

	suite.dir = dir
	suite.store = store
}

func (suite *StoreTestSuite) AfterTest(suiteName, testName string) {
	os.RemoveAll(suite.dir)
}

func (suite *StoreTestSuite) TestCreate() {
	err := suite.store.Create("test", nil, nil)
	assert.Nil(suite.T(), err)
}

func (suite *StoreTestSuite) TestGetUnknown() {
	meta, err := suite.store.Get("unknown")
	assert.Nil(suite.T(), meta)
	assert.Error(suite.T(), err)
}

func (suite *StoreTestSuite) TestGet() {
	err := suite.store.Create("test", TypeContext{
		Type:        "type",
		Description: "description",
	}, nil)
	assert.Nil(suite.T(), err)

	meta, err := suite.store.Get("test")
	assert.Nil(suite.T(), err)
	assert.NotNil(suite.T(), meta)
	assert.Equal(suite.T(), "test", meta.Name)

	m, ok := meta.Metadata.(TypeContext)
	assert.Equal(suite.T(), ok, true)
	assert.Equal(suite.T(), "description", m.Description)
	assert.Equal(suite.T(), "type", m.Type)
}
func (suite *StoreTestSuite) TestList() {
	err := suite.store.Create("test1", TypeContext{}, nil)
	assert.Nil(suite.T(), err)

	err = suite.store.Create("test2", TypeContext{}, nil)
	assert.Nil(suite.T(), err)

	contexts, err := suite.store.List()
	assert.Nil(suite.T(), err)

	require.Equal(suite.T(), len(contexts), 2)
	assert.Equal(suite.T(), contexts[0].Name, "test1")
	assert.Equal(suite.T(), contexts[1].Name, "test2")
}

func TestExampleTestSuite(t *testing.T) {
	suite.Run(t, new(StoreTestSuite))
}
