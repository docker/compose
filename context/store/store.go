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
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"

	"github.com/opencontainers/go-digest"
)

const (
	contextsDir = "contexts"
	metadataDir = "meta"
	metaFile    = "meta.json"
)

type contextStoreKey struct{}

func WithContextStore(ctx context.Context, store Store) context.Context {
	return context.WithValue(ctx, contextStoreKey{}, store)
}

func ContextStore(ctx context.Context) Store {
	s, _ := ctx.Value(contextStoreKey{}).(Store)
	return s
}

type Store interface {
	Get(name string) (*Metadata, error)
	Create(name string, data interface{}, endpoints map[string]interface{}) error
}

type store struct {
	root string
}

// New returns a configured context store
func New(root string) (Store, error) {
	cd := filepath.Join(root, contextsDir)
	if _, err := os.Stat(cd); os.IsNotExist(err) {
		if err = os.Mkdir(cd, 0755); err != nil {
			return nil, err
		}
	}
	m := filepath.Join(cd, metadataDir)
	if _, err := os.Stat(m); os.IsNotExist(err) {
		if err = os.Mkdir(m, 0755); err != nil {
			return nil, err
		}
	}

	return &store{
		root: root,
	}, nil
}

// Get returns the context with the given name
func (s *store) Get(name string) (*Metadata, error) {
	if name == "default" {
		return &Metadata{}, nil
	}

	meta := filepath.Join(s.root, contextsDir, metadataDir, contextdirOf(name), metaFile)
	bytes, err := ioutil.ReadFile(meta)
	if err != nil {
		return nil, err
	}

	r := &Metadata{
		Endpoints: make(map[string]interface{}),
	}

	typed := getter()
	if err := json.Unmarshal(bytes, typed); err != nil {
		return r, err
	}

	r.Metadata = reflect.ValueOf(typed).Elem().Interface()

	return r, nil
}

func (s *store) Create(name string, data interface{}, endpoints map[string]interface{}) error {
	dir := contextdirOf(name)
	metaDir := filepath.Join(s.root, contextsDir, metadataDir, dir)
	if _, err := os.Stat(metaDir); !os.IsNotExist(err) {
		return fmt.Errorf("Context %q already exists", name)
	}

	err := os.Mkdir(metaDir, 0755)
	if err != nil {
		return err
	}

	meta := Metadata{
		Name:      name,
		Metadata:  data,
		Endpoints: endpoints,
	}

	bytes, err := json.Marshal(&meta)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(filepath.Join(metaDir, metaFile), bytes, 0644)
}

func contextdirOf(name string) string {
	return digest.FromString(name).Encoded()
}

type Metadata struct {
	Name      string                 `json:",omitempty"`
	Metadata  interface{}            `json:",omitempty"`
	Endpoints map[string]interface{} `json:",omitempty"`
}

type TypeContext struct {
	Type        string
	Description string
}

func getter() interface{} {
	return &TypeContext{}
}
