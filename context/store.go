package context

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"

	"github.com/opencontainers/go-digest"
)

const (
	contextsDir = "contexts"
	metadataDir = "meta"
	metaFile    = "meta.json"
)

// ContextStoreDir returns the directory the docker contexts are stored in
func ContextStoreDir() string {
	return filepath.Join(ConfigDir, contextsDir)
}

type Metadata struct {
	Name      string                 `json:",omitempty"`
	Metadata  TypeContext            `json:",omitempty"`
	Endpoints map[string]interface{} `json:",omitempty"`
}

type TypeContext struct {
	Type string
}

func GetContext() (*Metadata, error) {
	config, err := LoadConfigFile()
	if err != nil {
		return nil, err
	}
	r := &Metadata{
		Endpoints: make(map[string]interface{}),
	}

	if ContextName == "" {
		ContextName = config.CurrentContext
	}
	if ContextName == "" || ContextName == "default" {
		r.Metadata.Type = "Moby"
		return r, nil
	}

	meta := filepath.Join(ConfigDir, contextsDir, metadataDir, contextdirOf(ContextName), metaFile)
	bytes, err := ioutil.ReadFile(meta)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(bytes, r); err != nil {
		return r, err
	}

	r.Name = ContextName
	return r, nil
}

func contextdirOf(name string) string {
	return digest.FromString(name).Encoded()
}
