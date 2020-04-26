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
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func setup(t *testing.T, cb func(*testing.T, Store)) {
	dir, err := ioutil.TempDir("", "store")
	assert.Nil(t, err)
	defer os.RemoveAll(dir)

	store, err := New(dir)
	assert.Nil(t, err)

	cb(t, store)
}

func TestGetUnknown(t *testing.T) {
	setup(t, func(t *testing.T, store Store) {
		meta, err := store.Get("unknown")
		assert.Nil(t, meta)
		assert.Error(t, err)
	})
}

func TestCreate(t *testing.T) {
	setup(t, func(t *testing.T, store Store) {
		err := store.Create("test", nil, nil)
		assert.Nil(t, err)
	})
}

func TestGet(t *testing.T) {
	setup(t, func(t *testing.T, store Store) {
		err := store.Create("test", TypeContext{
			Type:        "type",
			Description: "description",
		}, nil)
		assert.Nil(t, err)

		meta, err := store.Get("test")
		assert.Nil(t, err)
		assert.NotNil(t, meta)
		assert.Equal(t, "test", meta.Name)

		m, ok := meta.Metadata.(TypeContext)
		assert.Equal(t, ok, true)
		fmt.Printf("%#v\n", meta)
		assert.Equal(t, "description", m.Description)
		assert.Equal(t, "type", m.Type)
	})
}

func TestList(t *testing.T) {
	setup(t, func(t *testing.T, store Store) {
		err := store.Create("test1", TypeContext{}, nil)
		assert.Nil(t, err)

		err = store.Create("test2", TypeContext{}, nil)
		assert.Nil(t, err)

		contexts, err := store.List()
		assert.Nil(t, err)

		assert.Equal(t, len(contexts), 2)
		assert.Equal(t, contexts[0].Name, "test1")
		assert.Equal(t, contexts[1].Name, "test2")
	})
}
