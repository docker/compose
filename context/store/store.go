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

// Store
type Store interface {
	// Get returns the context with with name, it returns an error if the
	// context doesn't exist
	Get(name string) (*Metadata, error)
	// Create creates a new context, it returns an error if a context with the
	// same name exists already.
	Create(name string, data interface{}, endpoints map[string]interface{}) error
	// List returns the list of created contexts
	List() ([]*Metadata, error)
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
	return read(meta)
}

func read(meta string) (*Metadata, error) {
	bytes, err := ioutil.ReadFile(meta)
	if err != nil {
		return nil, err
	}

	var r untypedContextMetadata
	if err := json.Unmarshal(bytes, &r); err != nil {
		return nil, err
	}

	result := &Metadata{
		Name:      r.Name,
		Endpoints: r.Endpoints,
	}

	typed := getter()
	if err := json.Unmarshal(r.Metadata, typed); err != nil {
		return nil, err
	}

	result.Metadata = reflect.ValueOf(typed).Elem().Interface()

	return result, nil
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

func (s *store) List() ([]*Metadata, error) {
	root := filepath.Join(s.root, contextsDir, metadataDir)
	c, err := ioutil.ReadDir(root)
	if err != nil {
		return nil, err
	}

	var result []*Metadata
	for _, fi := range c {
		if fi.IsDir() {
			meta := filepath.Join(root, fi.Name(), metaFile)
			r, err := read(meta)
			if err != nil {
				return nil, err
			}
			result = append(result, r)
		}
	}

	return result, nil
}

func contextdirOf(name string) string {
	return digest.FromString(name).Encoded()
}

type Metadata struct {
	Name      string                 `json:",omitempty"`
	Metadata  interface{}            `json:",omitempty"`
	Endpoints map[string]interface{} `json:",omitempty"`
}

type untypedContextMetadata struct {
	Metadata  json.RawMessage        `json:"metadata,omitempty"`
	Endpoints map[string]interface{} `json:"endpoints,omitempty"`
	Name      string                 `json:"name,omitempty"`
}

type TypeContext struct {
	Type        string `json:",omitempty"`
	Description string `json:",omitempty"`
}

func getter() interface{} {
	return &TypeContext{}
}
