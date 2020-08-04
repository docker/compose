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

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"

	"github.com/docker/api/errdefs"
)

func testStore(t *testing.T) Store {
	d, err := ioutil.TempDir("", "store")
	assert.NilError(t, err)

	t.Cleanup(func() {
		_ = os.RemoveAll(d)
	})

	s, err := New(WithRoot(d))
	assert.NilError(t, err)

	return s
}

func TestCreate(t *testing.T) {
	s := testStore(t)
	err := s.Create("test", "test", "description", ContextMetadata{})
	assert.NilError(t, err)

	err = s.Create("test", "test", "descrsiption", ContextMetadata{})
	assert.Error(t, err, `context "test": already exists`)
	assert.Assert(t, errdefs.IsAlreadyExistsError(err))
}

func TestGetEndpoint(t *testing.T) {
	s := testStore(t)
	err := s.Create("aci", "aci", "description", AciContext{
		Location: "eu",
	})
	assert.NilError(t, err)

	var ctx AciContext
	err = s.GetEndpoint("aci", &ctx)
	assert.NilError(t, err)
	assert.Equal(t, ctx.Location, "eu")

	var exampleCtx ExampleContext
	err = s.GetEndpoint("aci", &exampleCtx)
	assert.Error(t, err, "wrong context type")
}

func TestGetUnknown(t *testing.T) {
	s := testStore(t)
	meta, err := s.Get("unknown")
	assert.Assert(t, cmp.Nil(meta))
	assert.Error(t, err, `context "unknown": not found`)
	assert.Assert(t, errdefs.IsNotFoundError(err))
}

func TestGet(t *testing.T) {
	s := testStore(t)
	err := s.Create("test", "type", "description", ContextMetadata{})
	assert.NilError(t, err)

	meta, err := s.Get("test")
	assert.NilError(t, err)
	assert.Assert(t, meta != nil)
	var m DockerContext
	if meta != nil {
		m = *meta
	}

	assert.Equal(t, m.Name, "test")
	assert.Equal(t, m.Metadata.Description, "description")
	assert.Equal(t, m.Type(), "type")
}

func TestRemoveNotFound(t *testing.T) {
	s := testStore(t)
	err := s.Remove("notfound")
	assert.Error(t, err, `context "notfound": not found`)
	assert.Assert(t, errdefs.IsNotFoundError(err))
}

func TestRemove(t *testing.T) {
	s := testStore(t)
	err := s.Create("testremove", "type", "description", ContextMetadata{})
	assert.NilError(t, err)

	meta, err := s.Get("testremove")
	assert.NilError(t, err)
	assert.Assert(t, meta != nil)

	err = s.Remove("testremove")
	assert.NilError(t, err)

	meta, err = s.Get("testremove")
	assert.Error(t, err, `context "testremove": not found`)
	assert.Assert(t, cmp.Nil(meta))

}
