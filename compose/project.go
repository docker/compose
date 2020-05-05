package compose

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/compose-spec/compose-go/loader"
	"github.com/compose-spec/compose-go/types"
	"github.com/sirupsen/logrus"
)

var supportedFilenames = []string{
	"compose.yml",
	"compose.yaml",
	"docker-compose.yml",
	"docker-compose.yaml",
}

// ProjectOptions configures a compose project
type ProjectOptions struct {
	Name        string
	WorkDir     string
	ConfigPaths []string
	Environment []string
}

// Project represents a compose project with a name
type Project struct {
	types.Config
	projectDir string
	Name       string `yaml:"-" json:"-"`
}

// ProjectFromOptions load a compose project based on given options
func ProjectFromOptions(options *ProjectOptions) (*Project, error) {
	configPath, err := getConfigPathFromOptions(options)
	if err != nil {
		return nil, err
	}

	configs, err := parseConfigs(configPath)
	if err != nil {
		return nil, err
	}

	name := options.Name
	if name == "" {
		r := regexp.MustCompile(`[^a-z0-9\\-_]+`)
		name = r.ReplaceAllString(strings.ToLower(filepath.Base(options.WorkDir)), "")
	}

	return newProject(types.ConfigDetails{
		WorkingDir:  options.WorkDir,
		ConfigFiles: configs,
		Environment: getAsEqualsMap(options.Environment),
	}, name)
}

func newProject(config types.ConfigDetails, name string) (*Project, error) {
	model, err := loader.Load(config)
	if err != nil {
		return nil, err
	}

	p := Project{
		Config:     *model,
		projectDir: config.WorkingDir,
		Name:       name,
	}
	return &p, nil
}

func getConfigPathFromOptions(options *ProjectOptions) ([]string, error) {
	var paths []string
	pwd := options.WorkDir

	if len(options.ConfigPaths) != 0 {
		for _, f := range options.ConfigPaths {
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

	for {
		var candidates []string
		for _, n := range supportedFilenames {
			f := filepath.Join(pwd, n)
			if _, err := os.Stat(f); err == nil {
				candidates = append(candidates, f)
			}
		}
		if len(candidates) > 0 {
			winner := candidates[0]
			if len(candidates) > 1 {
				logrus.Warnf("Found multiple config files with supported names: %s", strings.Join(candidates, ", "))
				logrus.Warnf("Using %s\n", winner)
			}
			return []string{winner}, nil
		}
		parent := filepath.Dir(pwd)
		if parent == pwd {
			return nil, fmt.Errorf("can't find a suitable configuration file in this directory or any parent. Is %q the right directory?", pwd)
		}
		pwd = parent
	}
}

func parseConfigs(configPaths []string) ([]types.ConfigFile, error) {
	var files []types.ConfigFile
	for _, f := range configPaths {
		var b []byte
		var err error
		if f == "-" {
			return []types.ConfigFile{}, errors.New("reading compose file from stdin is not supported")
		}
		if _, err := os.Stat(f); err != nil {
			return nil, err
		}
		b, err = ioutil.ReadFile(f)
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

// getAsEqualsMap split key=value formatted strings into a key : value map
func getAsEqualsMap(em []string) map[string]string {
	m := make(map[string]string)
	for _, v := range em {
		kv := strings.SplitN(v, "=", 2)
		m[kv[0]] = kv[1]
	}
	return m
}
