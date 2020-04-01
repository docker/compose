package utils

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/compose-spec/compose-go/loader"
	"github.com/compose-spec/compose-go/types"
	"github.com/prometheus/common/log"
)

var SupportedFilenames = []string{"compose.yaml", "compose.yml", "docker-compose.yml", "docker-compose.yaml"}

func GetConfigs(name string, configPaths []string) (string, []types.ConfigFile, error) {
	configPath, err := getConfigPaths(configPaths)
	if err != nil {
		return "", nil, err
	}

	if name == "" {
		name = os.Getenv("COMPOSE_PROJECT_NAME")
	}

	workingDir := filepath.Dir(configPath[0])

	if name == "" {
		r := regexp.MustCompile(`[^a-z0-9\\-_]+`)
		name = r.ReplaceAllString(strings.ToLower(filepath.Base(workingDir)), "")
	}

	configs, err := parseConfigs(configPath)
	if err != nil {
		return "", nil, err
	}
	return workingDir, configs, nil
}

func getConfigPaths(configPaths []string) ([]string, error) {
	paths := []string{}
	pwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	if len(configPaths) != 0 {
		for _, f := range configPaths {
			if f == "-" {
				paths = append(paths, f)
				continue
			}
			if !filepath.IsAbs(f) {
				f = filepath.Join(pwd, f)
			}
			if _, err := os.Stat(f); err != nil {
				return nil, err
			}
			paths = append(paths, f)
		}
		return paths, nil
	}

	sep := os.Getenv("COMPOSE_FILE_SEPARATOR")
	if sep == "" {
		sep = string(os.PathListSeparator)
	}
	f := os.Getenv("COMPOSE_FILE")
	if f != "" {
		return strings.Split(f, sep), nil
	}

	for {
		candidates := []string{}
		for _, n := range SupportedFilenames {
			f := filepath.Join(pwd, n)
			if _, err := os.Stat(f); err == nil {
				candidates = append(candidates, f)
			}
		}
		if len(candidates) > 0 {
			winner := candidates[0]
			if len(candidates) > 1 {
				log.Warnf("Found multiple config files with supported names: %s", strings.Join(candidates, ", "))
				log.Warnf("Using %s\n", winner)
			}
			return []string{winner}, nil
		}
		parent := filepath.Dir(pwd)
		if parent == pwd {
			return nil, fmt.Errorf("Can't find a suitable configuration file in this directory or any parent. Are you in the right directory?")
		}
		pwd = parent
	}
}

func parseConfigs(configPaths []string) ([]types.ConfigFile, error) {
	files := []types.ConfigFile{}
	for _, f := range configPaths {
		var (
			b   []byte
			err error
		)
		if f == "-" {
			b, err = ioutil.ReadAll(os.Stdin)
		} else {
			if _, err := os.Stat(f); err != nil {
				return nil, err
			}
			b, err = ioutil.ReadFile(f)
		}
		if err != nil {
			return nil, err
		}
		config, err := loader.ParseYAML(b)
		if err != nil {
			return nil, err
		}
		files = append(files, types.ConfigFile{Filename: f, Config: config})
	}
	return files, nil
}
