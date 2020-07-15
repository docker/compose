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
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"

	"github.com/opencontainers/go-digest"
	"github.com/pkg/errors"

	"github.com/docker/api/errdefs"
)

const (
	// DefaultContextName is an automatically generated local context
	DefaultContextName = "default"
	// DefaultContextType is the type for all moby contexts (not associated with cli backend)
	DefaultContextType = "moby"
	// AwsContextType is the type for ecs contexts (currently a CLI plugin, not associated with cli backend)
	AwsContextType = "aws"
	// AciContextType is the endpoint key in the context endpoints for an ACI
	// backend
	AciContextType = "aci"
	// LocalContextType is the endpoint key in the context endpoints for a new
	// local backend
	LocalContextType = "local"
	// ExampleContextType is the endpoint key in the context endpoints for an
	// example backend
	ExampleContextType = "example"
)

const (
	dockerEndpointKey = "docker"
	configDir         = ".docker"
	contextsDir       = "contexts"
	metadataDir       = "meta"
	metaFile          = "meta.json"
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
	Get(name string) (*DockerContext, error)
	// GetEndpoint sets the `v` parameter to the value of the endpoint for a
	// particular context type
	GetEndpoint(name string, v interface{}) error
	// Create creates a new context, it returns an error if a context with the
	// same name exists already.
	Create(name string, contextType string, description string, data interface{}) error
	// List returns the list of created contexts
	List() ([]*DockerContext, error)
	// Remove removes a context by name from the context store
	Remove(name string) error
	// ContextExists checks if a context already exists
	ContextExists(name string) bool
}

// Endpoint holds the Docker or the Kubernetes endpoint, they both have the
// `Host` property, only kubernetes will have the `DefaultNamespace`
type Endpoint struct {
	Host             string `json:",omitempty"`
	DefaultNamespace string `json:",omitempty"`
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

	root := filepath.Join(home, configDir)
	if err := createDirIfNotExist(root); err != nil {
		return nil, err
	}

	s := &store{
		root: root,
	}

	for _, opt := range opts {
		opt(s)
	}

	m := filepath.Join(s.root, contextsDir, metadataDir)
	if err := createDirIfNotExist(m); err != nil {
		return nil, err
	}

	return s, nil
}

// Get returns the context with the given name
func (s *store) Get(name string) (*DockerContext, error) {
	if name == "default" {
		return dockerDefaultContext()
	}
	meta := filepath.Join(s.root, contextsDir, metadataDir, contextDirOf(name), metaFile)
	m, err := read(meta)
	if os.IsNotExist(err) {
		return nil, errors.Wrap(errdefs.ErrNotFound, objectName(name))
	} else if err != nil {
		return nil, err
	}

	return m, nil
}

func (s *store) GetEndpoint(name string, data interface{}) error {
	meta, err := s.Get(name)
	if err != nil {
		return err
	}
	contextType := meta.Type()
	if _, ok := meta.Endpoints[contextType]; !ok {
		return errors.Wrapf(errdefs.ErrNotFound, "endpoint of type %q", contextType)
	}

	dstPtrValue := reflect.ValueOf(data)
	dstValue := reflect.Indirect(dstPtrValue)

	val := reflect.ValueOf(meta.Endpoints[contextType])
	valIndirect := reflect.Indirect(val)

	if dstValue.Type() != valIndirect.Type() {
		return errdefs.ErrWrongContextType
	}

	dstValue.Set(valIndirect)

	return nil
}

func read(meta string) (*DockerContext, error) {
	bytes, err := ioutil.ReadFile(meta)
	if err != nil {
		return nil, err
	}

	var metadata DockerContext
	if err := json.Unmarshal(bytes, &metadata); err != nil {
		return nil, err
	}

	metadata.Endpoints, err = toTypedEndpoints(metadata.Endpoints)
	if err != nil {
		return nil, err
	}

	return &metadata, nil
}

func toTypedEndpoints(endpoints map[string]interface{}) (map[string]interface{}, error) {
	result := map[string]interface{}{}
	for k, v := range endpoints {
		bytes, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		typeGetters := getters()
		typeGetter, ok := typeGetters[k]
		if !ok {
			typeGetter = func() interface{} {
				return &Endpoint{}
			}
		}

		val := typeGetter()
		err = json.Unmarshal(bytes, &val)
		if err != nil {
			return nil, err
		}

		result[k] = val
	}

	return result, nil
}

func (s *store) ContextExists(name string) bool {
	if name == DefaultContextName {
		return true
	}
	dir := contextDirOf(name)
	metaDir := filepath.Join(s.root, contextsDir, metadataDir, dir)
	if _, err := os.Stat(metaDir); !os.IsNotExist(err) {
		return true
	}
	return false
}

func (s *store) Create(name string, contextType string, description string, data interface{}) error {
	if s.ContextExists(name) {
		return errors.Wrap(errdefs.ErrAlreadyExists, objectName(name))
	}
	dir := contextDirOf(name)
	metaDir := filepath.Join(s.root, contextsDir, metadataDir, dir)

	err := os.Mkdir(metaDir, 0755)
	if err != nil {
		return err
	}

	meta := DockerContext{
		Name: name,
		Metadata: ContextMetadata{
			Type:        contextType,
			Description: description,
		},
		Endpoints: map[string]interface{}{
			(dockerEndpointKey): data,
			(contextType):       data,
		},
	}

	bytes, err := json.Marshal(&meta)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(filepath.Join(metaDir, metaFile), bytes, 0644)
}

func (s *store) List() ([]*DockerContext, error) {
	root := filepath.Join(s.root, contextsDir, metadataDir)
	c, err := ioutil.ReadDir(root)
	if err != nil {
		return nil, err
	}

	var result []*DockerContext
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

	// The default context is not stored in the store, it is in-memory only
	// so we need a special case for it.
	dockerDefault, err := dockerDefaultContext()
	if err != nil {
		return nil, err
	}

	result = append(result, dockerDefault)
	return result, nil
}

func (s *store) Remove(name string) error {
	if name == DefaultContextName {
		return errors.Wrap(errdefs.ErrForbidden, objectName(name))
	}
	dir := filepath.Join(s.root, contextsDir, metadataDir, contextDirOf(name))
	// Check if directory exists because os.RemoveAll returns nil if it doesn't
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return errors.Wrap(errdefs.ErrNotFound, objectName(name))
	}
	if err := os.RemoveAll(dir); err != nil {
		return errors.Wrapf(errdefs.ErrUnknown, "unable to remove %s: %s", objectName(name), err)
	}
	return nil
}

func contextDirOf(name string) string {
	return digest.FromString(name).Encoded()
}

func objectName(name string) string {
	return fmt.Sprintf("context %q", name)
}

func createDirIfNotExist(dir string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err = os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}

// Different context types managed by the store.
// TODO(rumpl): we should make this extensible in the future if we want to
// be able to manage other contexts.
func getters() map[string]func() interface{} {
	return map[string]func() interface{}{
		AciContextType: func() interface{} {
			return &AciContext{}
		},
		AwsContextType: func() interface{} {
			return &AwsContext{}
		},
		LocalContextType: func() interface{} {
			return &LocalContext{}
		},
		ExampleContextType: func() interface{} {
			return &ExampleContext{}
		},
	}
}
