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
	// Get returns the context with name, it returns an error if the  context
	// doesn't exist
	Get(name string, getter func() interface{}) (*Metadata, error)
	// GetType reurns the type of the context (docker, aci etc)
	GetType(meta *Metadata) string
	// Create creates a new context, it returns an error if a context with the
	// same name exists already.
	Create(name string, data TypedContext) error
	// List returns the list of created contexts
	List() ([]*Metadata, error)
}

type store struct {
	root string
}

type StoreOpt func(*store)

func WithRoot(root string) StoreOpt {
	return func(s *store) {
		s.root = root
	}
}

// New returns a configured context store
func New(opts ...StoreOpt) (Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	s := &store{
		root: filepath.Join(home, ".docker"),
	}
	for _, opt := range opts {
		opt(s)
	}
	cd := filepath.Join(s.root, contextsDir)
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
	return s, nil
}

// Get returns the context with the given name
func (s *store) Get(name string, getter func() interface{}) (*Metadata, error) {
	meta := filepath.Join(s.root, contextsDir, metadataDir, contextdirOf(name), metaFile)
	m, err := read(meta, getter)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("unknown context %q", name)
	} else if err != nil {
		return nil, err
	}

	return m, nil
}

func read(meta string, getter func() interface{}) (*Metadata, error) {
	bytes, err := ioutil.ReadFile(meta)
	if err != nil {
		return nil, err
	}

	var um untypedMetadata
	if err := json.Unmarshal(bytes, &um); err != nil {
		return nil, err
	}

	var uc untypedContext
	if err := json.Unmarshal(um.Metadata, &uc); err != nil {
		return nil, err
	}

	data, err := parse(uc.Data, getter)
	if err != nil {
		return nil, err
	}

	return &Metadata{
		Name:      um.Name,
		Endpoints: um.Endpoints,
		Metadata: TypedContext{
			Description: uc.Description,
			Type:        uc.Type,
			Data:        data,
		},
	}, nil
}

func parse(payload []byte, getter func() interface{}) (interface{}, error) {
	if getter == nil {
		var res map[string]interface{}
		if err := json.Unmarshal(payload, &res); err != nil {
			return nil, err
		}
		return res, nil
	}
	typed := getter()
	if err := json.Unmarshal(payload, &typed); err != nil {
		return nil, err
	}
	return reflect.ValueOf(typed).Elem().Interface(), nil
}

func (s *store) GetType(meta *Metadata) string {
	for k := range meta.Endpoints {
		if k != "docker" {
			return k
		}
	}
	return "docker"
}

func (s *store) Create(name string, data TypedContext) error {
	dir := contextdirOf(name)
	metaDir := filepath.Join(s.root, contextsDir, metadataDir, dir)
	if _, err := os.Stat(metaDir); !os.IsNotExist(err) {
		return fmt.Errorf("Context %q already exists", name)
	}

	err := os.Mkdir(metaDir, 0755)
	if err != nil {
		return err
	}

	if data.Data == nil {
		data.Data = DummyContext{}
	}

	meta := Metadata{
		Name:     name,
		Metadata: data,
		Endpoints: map[string]interface{}{
			"docker":    DummyContext{},
			(data.Type): DummyContext{},
		},
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
			r, err := read(meta, nil)
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

type DummyContext struct{}

type Metadata struct {
	Name      string                 `json:",omitempty"`
	Metadata  TypedContext           `json:",omitempty"`
	Endpoints map[string]interface{} `json:",omitempty"`
}

type untypedMetadata struct {
	Name      string                 `json:",omitempty"`
	Metadata  json.RawMessage        `json:",omitempty"`
	Endpoints map[string]interface{} `json:",omitempty"`
}

type untypedContext struct {
	Data        json.RawMessage `json:",omitempty"`
	Description string          `json:",omitempty"`
	Type        string          `json:",omitempty"`
}

type TypedContext struct {
	Type        string      `json:",omitempty"`
	Description string      `json:",omitempty"`
	Data        interface{} `json:",omitempty"`
}

type AciContext struct {
	SubscriptionID string `json:",omitempty"`
	Location       string `json:",omitempty"`
	ResourceGroup  string `json:",omitempty"`
}
