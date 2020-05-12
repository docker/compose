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

	"github.com/docker/api/errdefs"
	"github.com/opencontainers/go-digest"
	"github.com/pkg/errors"
)

const (
	contextsDir = "contexts"
	metadataDir = "meta"
	metaFile    = "meta.json"
)

type contextStoreKey struct{}

// WithContextStore adds the store to the context
func WithContextStore(ctx context.Context, store Store) context.Context {
	return context.WithValue(ctx, contextStoreKey{}, store)
}

// ContextStore returns the store from the context
func ContextStore(ctx context.Context) Store {
	s, _ := ctx.Value(contextStoreKey{}).(Store)
	return s
}

// Store is the context store
type Store interface {
	// Get returns the context with name, it returns an error if the  context
	// doesn't exist
	Get(name string, getter func() interface{}) (*Metadata, error)
	// GetType returns the type of the context (docker, aci etc)
	GetType(meta *Metadata) string
	// Create creates a new context, it returns an error if a context with the
	// same name exists already.
	Create(name string, data TypedContext) error
	// List returns the list of created contexts
	List() ([]*Metadata, error)
	// Remove removes a context by name from the context store
	Remove(name string) error
}

type store struct {
	root string
}

// Opt is a functional option for the store
type Opt func(*store)

// WithRoot sets a new root to the store
func WithRoot(root string) Opt {
	return func(s *store) {
		s.root = root
	}
}

// New returns a configured context store with $HOME/.docker as root
func New(opts ...Opt) (Store, error) {
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
		return nil, errors.Wrap(errdefs.ErrNotFound, objectName(name))
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
		return errors.Wrap(errdefs.ErrAlreadyExists, objectName(name))
	}

	err := os.Mkdir(metaDir, 0755)
	if err != nil {
		return err
	}

	if data.Data == nil {
		data.Data = dummyContext{}
	}

	meta := Metadata{
		Name:     name,
		Metadata: data,
		Endpoints: map[string]interface{}{
			"docker":    dummyContext{},
			(data.Type): dummyContext{},
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

func (s *store) Remove(name string) error {
	dir := filepath.Join(s.root, contextsDir, metadataDir, contextdirOf(name))
	// Check if directory exists because os.RemoveAll returns nil if it doesn't
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return errors.Wrap(errdefs.ErrNotFound, objectName(name))
	}
	if err := os.RemoveAll(dir); err != nil {
		return errors.Wrapf(errdefs.ErrUnknown, "unable to remove %s: %s", objectName(name), err)
	}
	return nil
}

func contextdirOf(name string) string {
	return digest.FromString(name).Encoded()
}

func objectName(name string) string {
	return fmt.Sprintf("context %q", name)
}

type dummyContext struct{}

// Metadata represents the docker context metadata
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

// TypedContext is a context with a type (moby, aci, etc...)
type TypedContext struct {
	Type        string      `json:",omitempty"`
	Description string      `json:",omitempty"`
	Data        interface{} `json:",omitempty"`
}

// AciContext is the context for ACI
type AciContext struct {
	SubscriptionID string `json:",omitempty"`
	Location       string `json:",omitempty"`
	ResourceGroup  string `json:",omitempty"`
}
