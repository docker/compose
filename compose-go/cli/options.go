/*
   Copyright 2020 The Compose Specification Authors.

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

package cli

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/op/go-logging.v1"
	"gopkg.in/yaml.v3"

	"github.com/compose-spec/compose-go/consts"
	"github.com/compose-spec/compose-go/dotenv"
	"github.com/compose-spec/compose-go/errdefs"
	"github.com/compose-spec/compose-go/loader"
	"github.com/compose-spec/compose-go/types"
	"github.com/compose-spec/compose-go/utils"
	"github.com/mikefarah/yq/v4/pkg/yqlib"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ProjectOptions groups the command line options recommended for a Compose implementation
type ProjectOptions struct {
	Name        string
	WorkingDir  string
	ConfigPaths []string
	Environment map[string]string
	EnvFile     string
	loadOptions []func(*loader.Options)
}

type ProjectOptionsFn func(*ProjectOptions) error

// NewProjectOptions creates ProjectOptions
func NewProjectOptions(configs []string, opts ...ProjectOptionsFn) (*ProjectOptions, error) {
	options := &ProjectOptions{
		ConfigPaths: configs,
		Environment: map[string]string{},
	}
	for _, o := range opts {
		err := o(options)
		if err != nil {
			return nil, err
		}
	}
	return options, nil
}

// WithName defines ProjectOptions' name
func WithName(name string) ProjectOptionsFn {
	return func(o *ProjectOptions) error {
		o.Name = name
		return nil
	}
}

// WithWorkingDirectory defines ProjectOptions' working directory
func WithWorkingDirectory(wd string) ProjectOptionsFn {
	return func(o *ProjectOptions) error {
		if wd == "" {
			return nil
		}
		abs, err := filepath.Abs(wd)
		if err != nil {
			return err
		}
		o.WorkingDir = abs
		return nil
	}
}

// WithConfigFileEnv allow to set compose config file paths by COMPOSE_FILE environment variable
func WithConfigFileEnv(o *ProjectOptions) error {
	if len(o.ConfigPaths) > 0 {
		return nil
	}
	sep := o.Environment[consts.ComposePathSeparator]
	if sep == "" {
		sep = string(os.PathListSeparator)
	}
	f, ok := o.Environment[consts.ComposeFilePath]
	if ok {
		paths, err := absolutePaths(strings.Split(f, sep))
		o.ConfigPaths = paths
		return err
	}
	return nil
}

// WithDefaultConfigPath searches for default config files from working directory
func WithDefaultConfigPath(o *ProjectOptions) error {
	if len(o.ConfigPaths) > 0 {
		return nil
	}
	pwd, err := o.GetWorkingDir()
	if err != nil {
		return err
	}
	for {
		candidates := findFiles(DefaultFileNames, pwd)
		if len(candidates) > 0 {
			winner := candidates[0]
			if len(candidates) > 1 {
				logrus.Warnf("Found multiple config files with supported names: %s", strings.Join(candidates, ", "))
				logrus.Warnf("Using %s", winner)
			}
			o.ConfigPaths = append(o.ConfigPaths, winner)

			overrides := findFiles(DefaultOverrideFileNames, pwd)
			if len(overrides) > 0 {
				if len(overrides) > 1 {
					logrus.Warnf("Found multiple override files with supported names: %s", strings.Join(overrides, ", "))
					logrus.Warnf("Using %s", overrides[0])
				}
				o.ConfigPaths = append(o.ConfigPaths, overrides[0])
			}
			return nil
		}
		parent := filepath.Dir(pwd)
		if parent == pwd {
			// no config file found, but that's not a blocker if caller only needs project name
			return nil
		}
		pwd = parent
	}
}

// WithEnv defines a key=value set of variables used for compose file interpolation
func WithEnv(env []string) ProjectOptionsFn {
	return func(o *ProjectOptions) error {
		for k, v := range getAsEqualsMap(env) {
			o.Environment[k] = v
		}
		return nil
	}
}

// WithDiscardEnvFiles sets discards the `env_file` section after resolving to
// the `environment` section
func WithDiscardEnvFile(o *ProjectOptions) error {
	o.loadOptions = append(o.loadOptions, loader.WithDiscardEnvFiles)
	return nil
}

// WithLoadOptions provides a hook to control how compose files are loaded
func WithLoadOptions(loadOptions ...func(*loader.Options)) ProjectOptionsFn {
	return func(o *ProjectOptions) error {
		o.loadOptions = append(o.loadOptions, loadOptions...)
		return nil
	}
}

// WithOsEnv imports environment variables from OS
func WithOsEnv(o *ProjectOptions) error {
	for k, v := range getAsEqualsMap(os.Environ()) {
		o.Environment[k] = v
	}
	return nil
}

// WithEnvFile set an alternate env file
func WithEnvFile(file string) ProjectOptionsFn {
	return func(options *ProjectOptions) error {
		options.EnvFile = file
		return nil
	}
}

// WithDotEnv imports environment variables from .env file
func WithDotEnv(o *ProjectOptions) error {
	dotEnvFile := o.EnvFile
	if dotEnvFile == "" {
		wd, err := o.GetWorkingDir()
		if err != nil {
			return err
		}
		dotEnvFile = filepath.Join(wd, ".env")
	}
	abs, err := filepath.Abs(dotEnvFile)
	if err != nil {
		return err
	}
	dotEnvFile = abs

	s, err := os.Stat(dotEnvFile)
	if os.IsNotExist(err) {
		if o.EnvFile != "" {
			return errors.Errorf("Couldn't find env file: %s", o.EnvFile)
		}
		return nil
	}
	if err != nil {
		return err
	}

	if s.IsDir() {
		if o.EnvFile == "" {
			return nil
		}
		return errors.Errorf("%s is a directory", dotEnvFile)
	}

	file, err := os.Open(dotEnvFile)
	if err != nil {
		return err
	}
	defer file.Close()

	notInEnvSet := make(map[string]interface{})
	env, err := dotenv.ParseWithLookup(file, func(k string) (string, bool) {
		v, ok := os.LookupEnv(k)
		if !ok {
			notInEnvSet[k] = nil
			return "", true
		}
		return v, true
	})
	if err != nil {
		return err
	}
	for k, v := range env {
		if _, ok := notInEnvSet[k]; ok {
			continue
		}
		o.Environment[k] = v
	}
	return nil
}

// WithInterpolation set ProjectOptions to enable/skip interpolation
func WithInterpolation(interpolation bool) ProjectOptionsFn {
	return func(o *ProjectOptions) error {
		o.loadOptions = append(o.loadOptions, func(options *loader.Options) {
			options.SkipInterpolation = !interpolation
		})
		return nil
	}
}

// WithNormalization set ProjectOptions to enable/skip normalization
func WithNormalization(normalization bool) ProjectOptionsFn {
	return func(o *ProjectOptions) error {
		o.loadOptions = append(o.loadOptions, func(options *loader.Options) {
			options.SkipNormalization = !normalization
		})
		return nil
	}
}

// WithResolvedPaths set ProjectOptions to enable paths resolution
func WithResolvedPaths(resolve bool) ProjectOptionsFn {
	return func(o *ProjectOptions) error {
		o.loadOptions = append(o.loadOptions, func(options *loader.Options) {
			options.ResolvePaths = resolve
		})
		return nil
	}
}

// DefaultFileNames defines the Compose file names for auto-discovery (in order of preference)
var DefaultFileNames = []string{
	"compose.xml", "docker-compose.xml",
	"compose.yaml", "compose.yml", "docker-compose.yml", "docker-compose.yaml",
}

// DefaultOverrideFileNames defines the Compose override file names for auto-discovery (in order of preference)
var DefaultOverrideFileNames = []string{
	"compose.override.xml", "docker-compose.override.xml",
	"compose.override.yml", "compose.override.yaml", "docker-compose.override.yml", "docker-compose.override.yaml",
}

func (o ProjectOptions) GetWorkingDir() (string, error) {
	if o.WorkingDir != "" {
		return o.WorkingDir, nil
	}
	for _, path := range o.ConfigPaths {
		if path != "-" {
			absPath, err := filepath.Abs(path)
			if err != nil {
				return "", err
			}
			return filepath.Dir(absPath), nil
		}
	}
	return os.Getwd()
}

// ProjectFromOptions load a compose project based on command line options
func ProjectFromOptions(options *ProjectOptions) (*types.Project, error) {
	configPaths, err := getConfigPathsFromOptions(options)
	if err != nil {
		return nil, err
	}

	var configs []types.ConfigFile
	for _, f := range configPaths {
		var b []byte
		if f == "-" {
			b, err = ioutil.ReadAll(os.Stdin)
			if err != nil {
				return nil, err
			}
		} else {
			f, err := filepath.Abs(f)
			if err != nil {
				return nil, err
			}
			b, err = ioutil.ReadFile(f)
			if err != nil {
				return nil, err
			}
			if strings.EqualFold(filepath.Ext(f), ".xml") {
				l := yqlib.GetLogger()
				lvlBackend := logging.AddModuleLevel(logging.InitForTesting(logging.ERROR))
				lvlBackend.SetLevel(logging.ERROR, "")
				l.SetBackend(lvlBackend)

				dec := yqlib.NewXMLDecoder("+", "+content", false)
				dec.Init(strings.NewReader(string(b)))

				var node yaml.Node
				err = dec.Decode(&node)
				if err != nil {
					return nil, errors.Wrap(err, "decoding xml")
				}

				ydec := yqlib.NewYamlEncoder(4, false, true, true)
				out := new(bytes.Buffer)
				err = ydec.Encode(out, &node)
				if err != nil {
					return nil, errors.Wrap(err, "encoding yaml")
				}
				b = out.Bytes()
			}
		}
		configs = append(configs, types.ConfigFile{
			Filename: f,
			Content:  b,
		})
	}

	workingDir, err := options.GetWorkingDir()
	if err != nil {
		return nil, err
	}
	absWorkingDir, err := filepath.Abs(workingDir)
	if err != nil {
		return nil, err
	}

	options.loadOptions = append(options.loadOptions,
		withNamePrecedenceLoad(absWorkingDir, options),
		withConvertWindowsPaths(options))

	project, err := loader.Load(types.ConfigDetails{
		ConfigFiles: configs,
		WorkingDir:  workingDir,
		Environment: options.Environment,
	}, options.loadOptions...)
	if err != nil {
		return nil, err
	}

	project.ComposeFiles = configPaths
	return project, nil
}

func withNamePrecedenceLoad(absWorkingDir string, options *ProjectOptions) func(*loader.Options) {
	return func(opts *loader.Options) {
		if options.Name != "" {
			opts.SetProjectName(options.Name, true)
		} else if nameFromEnv, ok := options.Environment[consts.ComposeProjectName]; ok && nameFromEnv != "" {
			opts.SetProjectName(nameFromEnv, true)
		} else {
			opts.SetProjectName(filepath.Base(absWorkingDir), false)
		}
	}
}

func withConvertWindowsPaths(options *ProjectOptions) func(*loader.Options) {
	return func(o *loader.Options) {
		o.ConvertWindowsPaths = utils.StringToBool(options.Environment["COMPOSE_CONVERT_WINDOWS_PATHS"])
	}
}

// getConfigPathsFromOptions retrieves the config files for project based on project options
func getConfigPathsFromOptions(options *ProjectOptions) ([]string, error) {
	if len(options.ConfigPaths) != 0 {
		return absolutePaths(options.ConfigPaths)
	}
	return nil, errors.Wrap(errdefs.ErrNotFound, "no configuration file provided")
}

func findFiles(names []string, pwd string) []string {
	candidates := []string{}
	for _, n := range names {
		f := filepath.Join(pwd, n)
		if _, err := os.Stat(f); err == nil {
			candidates = append(candidates, f)
		}
	}
	return candidates
}

func absolutePaths(p []string) ([]string, error) {
	var paths []string
	for _, f := range p {
		if f == "-" {
			paths = append(paths, f)
			continue
		}
		abs, err := filepath.Abs(f)
		if err != nil {
			return nil, err
		}
		f = abs
		if _, err := os.Stat(f); err != nil {
			return nil, err
		}
		paths = append(paths, f)
	}
	return paths, nil
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

// getAsEqualsMap format a key : value map into key=value strings
func getAsStringList(em map[string]string) []string {
	m := make([]string, 0, len(em))
	for k, v := range em {
		m = append(m, fmt.Sprintf("%s=%s", k, v))
	}
	return m
}
