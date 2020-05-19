package compose

import (
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

type Project struct {
	types.Config
	projectDir string
	Name       string `yaml:"-" json:"-"`
}

func NewProject(config types.ConfigDetails, name string) (*Project, error) {
	model, err := loader.Load(config)
	if err != nil {
		return nil, err
	}

	err = Normalize(model)
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

// projectFromOptions load a compose project based on command line options
func ProjectFromOptions(options *ProjectOptions) (*Project, error) {
	configPath, err := getConfigPathFromOptions(options)
	if err != nil {
		return nil, err
	}

	name := options.Name
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
		return nil, err
	}

	return NewProject(types.ConfigDetails{
		WorkingDir:  workingDir,
		ConfigFiles: configs,
		Environment: environment(),
	}, name)
}

func getConfigPathFromOptions(options *ProjectOptions) ([]string, error) {
	paths := []string{}
	pwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

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
				logrus.Warnf("Found multiple config files with supported names: %s", strings.Join(candidates, ", "))
				logrus.Warnf("Using %s\n", winner)
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

var SupportedFilenames = []string{"compose.yaml", "compose.yml", "docker-compose.yml", "docker-compose.yaml"}

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

func environment() map[string]string {
	return getAsEqualsMap(os.Environ())
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
