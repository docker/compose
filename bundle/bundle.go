package bundle

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/opencontainers/runtime-spec/specs-go"
)

const configName = "config.json"

func New(path string, s *specs.Spec) (*Bundle, error) {
	if err := os.Mkdir(path, 0700); err != nil {
		return nil, err
	}
	b := &Bundle{
		Path: path,
	}
	if err := os.Mkdir(filepath.Join(path, "rootfs"), 0700); err != nil {
		b.Delete()
		return nil, err
	}
	f, err := os.Create(filepath.Join(path, configName))
	if err != nil {
		b.Delete()
		return nil, err
	}
	err = json.NewEncoder(f).Encode(s)
	f.Close()
	if err != nil {
		b.Delete()
		return nil, err
	}
	return b, nil
}

func Load(path string) (*Bundle, error) {
	// TODO: do validation
	return &Bundle{
		Path: path,
	}, nil
}

type Bundle struct {
	Path string
}

func (b *Bundle) Config() (*specs.Spec, error) {
	var s specs.Spec
	f, err := os.Open(filepath.Join(b.Path, configName))
	if err != nil {
		return nil, err
	}
	err = json.NewDecoder(f).Decode(&s)
	f.Close()
	return &s, err
}

func (b *Bundle) Delete() error {
	return os.RemoveAll(b.Path)
}
