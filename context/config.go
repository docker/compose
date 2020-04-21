package context

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func LoadConfigFile() (*ConfigFile, error) {
	filename := filepath.Join(ConfigDir, ConfigFileName)
	configFile := &ConfigFile{
		Filename: filename,
	}

	if _, err := os.Stat(filename); err == nil {
		file, err := os.Open(filename)
		if err != nil {
			return nil, fmt.Errorf("can't read %s: %w", filename, err)
		}
		defer file.Close()
		err = json.NewDecoder(file).Decode(&configFile)
		if err != nil {
			err = fmt.Errorf("can't read %s: %w", filename, err)
		}
		return configFile, err
	} else if !os.IsNotExist(err) {
		// if file is there but we can't stat it for any reason other
		// than it doesn't exist then stop
		return nil, fmt.Errorf("can't read %s: %w", filename, err)
	}
	return configFile, nil
}

type ConfigFile struct {
	Filename       string `json:"-"` // Note: for internal use only
	CurrentContext string `json:"currentContext,omitempty"`
}
