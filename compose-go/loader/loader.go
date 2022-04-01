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

package loader

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/compose-spec/compose-go/consts"
	"github.com/compose-spec/compose-go/dotenv"
	interp "github.com/compose-spec/compose-go/interpolation"
	"github.com/compose-spec/compose-go/schema"
	"github.com/compose-spec/compose-go/template"
	"github.com/compose-spec/compose-go/types"
	"github.com/docker/go-units"
	"github.com/mattn/go-shellwords"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

// Options supported by Load
type Options struct {
	// Skip schema validation
	SkipValidation bool
	// Skip interpolation
	SkipInterpolation bool
	// Skip normalization
	SkipNormalization bool
	// Resolve paths
	ResolvePaths bool
	// Convert Windows paths
	ConvertWindowsPaths bool
	// Skip consistency check
	SkipConsistencyCheck bool
	// Skip extends
	SkipExtends bool
	// Interpolation options
	Interpolate *interp.Options
	// Discard 'env_file' entries after resolving to 'environment' section
	discardEnvFiles bool
	// Set project projectName
	projectName string
	// Indicates when the projectName was imperatively set or guessed from path
	projectNameImperativelySet bool
}

func (o *Options) SetProjectName(name string, imperativelySet bool) {
	o.projectName = normalizeProjectName(name)
	o.projectNameImperativelySet = imperativelySet
}

func (o Options) GetProjectName() (string, bool) {
	return o.projectName, o.projectNameImperativelySet
}

// serviceRef identifies a reference to a service. It's used to detect cyclic
// references in "extends".
type serviceRef struct {
	filename string
	service  string
}

type cycleTracker struct {
	loaded []serviceRef
}

func (ct *cycleTracker) Add(filename, service string) error {
	toAdd := serviceRef{filename: filename, service: service}
	for _, loaded := range ct.loaded {
		if toAdd == loaded {
			// Create an error message of the form:
			// Circular reference:
			//   service-a in docker-compose.yml
			//   extends service-b in docker-compose.yml
			//   extends service-a in docker-compose.yml
			errLines := []string{
				"Circular reference:",
				fmt.Sprintf("  %s in %s", ct.loaded[0].service, ct.loaded[0].filename),
			}
			for _, service := range append(ct.loaded[1:], toAdd) {
				errLines = append(errLines, fmt.Sprintf("  extends %s in %s", service.service, service.filename))
			}

			return errors.New(strings.Join(errLines, "\n"))
		}
	}

	ct.loaded = append(ct.loaded, toAdd)
	return nil
}

// WithDiscardEnvFiles sets the Options to discard the `env_file` section after resolving to
// the `environment` section
func WithDiscardEnvFiles(opts *Options) {
	opts.discardEnvFiles = true
}

// WithSkipValidation sets the Options to skip validation when loading sections
func WithSkipValidation(opts *Options) {
	opts.SkipValidation = true
}

// ParseYAML reads the bytes from a file, parses the bytes into a mapping
// structure, and returns it.
func ParseYAML(source []byte) (map[string]interface{}, error) {
	var cfg interface{}
	if err := yaml.Unmarshal(source, &cfg); err != nil {
		return nil, err
	}
	cfgMap, ok := cfg.(map[interface{}]interface{})
	if !ok {
		return nil, errors.Errorf("Top-level object must be a mapping")
	}
	converted, err := convertToStringKeysRecursive(cfgMap, "")
	if err != nil {
		return nil, err
	}
	return converted.(map[string]interface{}), nil
}

// Load reads a ConfigDetails and returns a fully loaded configuration
func Load(configDetails types.ConfigDetails, options ...func(*Options)) (*types.Project, error) {
	if len(configDetails.ConfigFiles) < 1 {
		return nil, errors.Errorf("No files specified")
	}

	opts := &Options{
		Interpolate: &interp.Options{
			Substitute:      template.Substitute,
			LookupValue:     configDetails.LookupEnv,
			TypeCastMapping: interpolateTypeCastMapping,
		},
	}

	for _, op := range options {
		op(opts)
	}

	var configs []*types.Config
	for i, file := range configDetails.ConfigFiles {
		configDict := file.Config
		if configDict == nil {
			dict, err := parseConfig(file.Content, opts)
			if err != nil {
				return nil, err
			}
			configDict = dict
			file.Config = dict
			configDetails.ConfigFiles[i] = file
		}

		if !opts.SkipValidation {
			if err := schema.Validate(configDict); err != nil {
				return nil, err
			}
		}

		configDict = groupXFieldsIntoExtensions(configDict)

		cfg, err := loadSections(file.Filename, configDict, configDetails, opts)
		if err != nil {
			return nil, err
		}
		if opts.discardEnvFiles {
			for i := range cfg.Services {
				cfg.Services[i].EnvFile = nil
			}
		}

		configs = append(configs, cfg)
	}

	model, err := merge(configs)
	if err != nil {
		return nil, err
	}

	for _, s := range model.Services {
		var newEnvFiles types.StringList
		for _, ef := range s.EnvFile {
			newEnvFiles = append(newEnvFiles, absPath(configDetails.WorkingDir, ef))
		}
		s.EnvFile = newEnvFiles
	}

	projectName, projectNameImperativelySet := opts.GetProjectName()
	model.Name = normalizeProjectName(model.Name)
	if !projectNameImperativelySet && model.Name != "" {
		projectName = model.Name
	}

	if projectName != "" {
		configDetails.Environment[consts.ComposeProjectName] = projectName
	}
	project := &types.Project{
		Name:        projectName,
		WorkingDir:  configDetails.WorkingDir,
		Services:    model.Services,
		Networks:    model.Networks,
		Volumes:     model.Volumes,
		Secrets:     model.Secrets,
		Configs:     model.Configs,
		Environment: configDetails.Environment,
		Extensions:  model.Extensions,
	}

	if !opts.SkipNormalization {
		err = normalize(project, opts.ResolvePaths)
		if err != nil {
			return nil, err
		}
	}

	if !opts.SkipConsistencyCheck {
		err = checkConsistency(project)
		if err != nil {
			return nil, err
		}
	}

	return project, nil
}

func normalizeProjectName(s string) string {
	r := regexp.MustCompile("[a-z0-9_-]")
	s = strings.ToLower(s)
	s = strings.Join(r.FindAllString(s, -1), "")
	return strings.TrimLeft(s, "_-")
}

func parseConfig(b []byte, opts *Options) (map[string]interface{}, error) {
	yml, err := ParseYAML(b)
	if err != nil {
		return nil, err
	}
	if !opts.SkipInterpolation {
		return interp.Interpolate(yml, *opts.Interpolate)
	}
	return yml, err
}

func groupXFieldsIntoExtensions(dict map[string]interface{}) map[string]interface{} {
	extras := map[string]interface{}{}
	for key, value := range dict {
		if strings.HasPrefix(key, "x-") {
			extras[key] = value
			delete(dict, key)
		}
		if d, ok := value.(map[string]interface{}); ok {
			dict[key] = groupXFieldsIntoExtensions(d)
		}
	}
	if len(extras) > 0 {
		dict["extensions"] = extras
	}
	return dict
}

func loadSections(filename string, config map[string]interface{}, configDetails types.ConfigDetails, opts *Options) (*types.Config, error) {
	var err error
	cfg := types.Config{
		Filename: filename,
	}
	name := ""
	if n, ok := config["name"]; ok {
		name, ok = n.(string)
		if !ok {
			return nil, errors.New("project name must be a string")
		}
	}
	cfg.Name = name
	cfg.Services, err = LoadServices(filename, getSection(config, "services"), configDetails.WorkingDir, configDetails.LookupEnv, opts)
	if err != nil {
		return nil, err
	}

	cfg.Networks, err = LoadNetworks(getSection(config, "networks"))
	if err != nil {
		return nil, err
	}
	cfg.Volumes, err = LoadVolumes(getSection(config, "volumes"))
	if err != nil {
		return nil, err
	}
	cfg.Secrets, err = LoadSecrets(getSection(config, "secrets"), configDetails, opts.ResolvePaths)
	if err != nil {
		return nil, err
	}
	cfg.Configs, err = LoadConfigObjs(getSection(config, "configs"), configDetails, opts.ResolvePaths)
	if err != nil {
		return nil, err
	}
	extensions := getSection(config, "extensions")
	if len(extensions) > 0 {
		cfg.Extensions = extensions
	}
	return &cfg, nil
}

func getSection(config map[string]interface{}, key string) map[string]interface{} {
	section, ok := config[key]
	if !ok {
		return make(map[string]interface{})
	}
	return section.(map[string]interface{})
}

// ForbiddenPropertiesError is returned when there are properties in the Compose
// file that are forbidden.
type ForbiddenPropertiesError struct {
	Properties map[string]string
}

func (e *ForbiddenPropertiesError) Error() string {
	return "Configuration contains forbidden properties"
}

// Transform converts the source into the target struct with compose types transformer
// and the specified transformers if any.
func Transform(source interface{}, target interface{}, additionalTransformers ...Transformer) error {
	data := mapstructure.Metadata{}
	config := &mapstructure.DecoderConfig{
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			createTransformHook(additionalTransformers...),
			mapstructure.StringToTimeDurationHookFunc()),
		Result:   target,
		Metadata: &data,
	}
	decoder, err := mapstructure.NewDecoder(config)
	if err != nil {
		return err
	}
	return decoder.Decode(source)
}

// TransformerFunc defines a function to perform the actual transformation
type TransformerFunc func(interface{}) (interface{}, error)

// Transformer defines a map to type transformer
type Transformer struct {
	TypeOf reflect.Type
	Func   TransformerFunc
}

func createTransformHook(additionalTransformers ...Transformer) mapstructure.DecodeHookFuncType {
	transforms := map[reflect.Type]func(interface{}) (interface{}, error){
		reflect.TypeOf(types.External{}):                         transformExternal,
		reflect.TypeOf(types.HealthCheckTest{}):                  transformHealthCheckTest,
		reflect.TypeOf(types.ShellCommand{}):                     transformShellCommand,
		reflect.TypeOf(types.StringList{}):                       transformStringList,
		reflect.TypeOf(map[string]string{}):                      transformMapStringString,
		reflect.TypeOf(types.UlimitsConfig{}):                    transformUlimits,
		reflect.TypeOf(types.UnitBytes(0)):                       transformSize,
		reflect.TypeOf([]types.ServicePortConfig{}):              transformServicePort,
		reflect.TypeOf(types.ServiceSecretConfig{}):              transformFileReferenceConfig,
		reflect.TypeOf(types.ServiceConfigObjConfig{}):           transformFileReferenceConfig,
		reflect.TypeOf(types.StringOrNumberList{}):               transformStringOrNumberList,
		reflect.TypeOf(map[string]*types.ServiceNetworkConfig{}): transformServiceNetworkMap,
		reflect.TypeOf(types.Mapping{}):                          transformMappingOrListFunc("=", false),
		reflect.TypeOf(types.MappingWithEquals{}):                transformMappingOrListFunc("=", true),
		reflect.TypeOf(types.Labels{}):                           transformMappingOrListFunc("=", false),
		reflect.TypeOf(types.MappingWithColon{}):                 transformMappingOrListFunc(":", false),
		reflect.TypeOf(types.HostsList{}):                        transformListOrMappingFunc(":", false),
		reflect.TypeOf(types.ServiceVolumeConfig{}):              transformServiceVolumeConfig,
		reflect.TypeOf(types.BuildConfig{}):                      transformBuildConfig,
		reflect.TypeOf(types.Duration(0)):                        transformStringToDuration,
		reflect.TypeOf(types.DependsOnConfig{}):                  transformDependsOnConfig,
		reflect.TypeOf(types.ExtendsConfig{}):                    transformExtendsConfig,
		reflect.TypeOf(types.DeviceRequest{}):                    transformServiceDeviceRequest,
		reflect.TypeOf(types.SSHConfig{}):                        transformSSHConfig,
	}

	for _, transformer := range additionalTransformers {
		transforms[transformer.TypeOf] = transformer.Func
	}

	return func(_ reflect.Type, target reflect.Type, data interface{}) (interface{}, error) {
		transform, ok := transforms[target]
		if !ok {
			return data, nil
		}
		return transform(data)
	}
}

// keys need to be converted to strings for jsonschema
func convertToStringKeysRecursive(value interface{}, keyPrefix string) (interface{}, error) {
	if mapping, ok := value.(map[interface{}]interface{}); ok {
		dict := make(map[string]interface{})
		for key, entry := range mapping {
			str, ok := key.(string)
			if !ok {
				return nil, formatInvalidKeyError(keyPrefix, key)
			}
			var newKeyPrefix string
			if keyPrefix == "" {
				newKeyPrefix = str
			} else {
				newKeyPrefix = fmt.Sprintf("%s.%s", keyPrefix, str)
			}
			convertedEntry, err := convertToStringKeysRecursive(entry, newKeyPrefix)
			if err != nil {
				return nil, err
			}
			dict[str] = convertedEntry
		}
		return dict, nil
	}
	if list, ok := value.([]interface{}); ok {
		var convertedList []interface{}
		for index, entry := range list {
			newKeyPrefix := fmt.Sprintf("%s[%d]", keyPrefix, index)
			convertedEntry, err := convertToStringKeysRecursive(entry, newKeyPrefix)
			if err != nil {
				return nil, err
			}
			convertedList = append(convertedList, convertedEntry)
		}
		return convertedList, nil
	}
	return value, nil
}

func formatInvalidKeyError(keyPrefix string, key interface{}) error {
	var location string
	if keyPrefix == "" {
		location = "at top level"
	} else {
		location = fmt.Sprintf("in %s", keyPrefix)
	}
	return errors.Errorf("Non-string key %s: %#v", location, key)
}

// LoadServices produces a ServiceConfig map from a compose file Dict
// the servicesDict is not validated if directly used. Use Load() to enable validation
func LoadServices(filename string, servicesDict map[string]interface{}, workingDir string, lookupEnv template.Mapping, opts *Options) ([]types.ServiceConfig, error) {
	var services []types.ServiceConfig

	x, ok := servicesDict["extensions"]
	if ok {
		// as a top-level attribute, "services" doesn't support extensions, and a service can be named `x-foo`
		for k, v := range x.(map[string]interface{}) {
			servicesDict[k] = v
		}
	}

	for name := range servicesDict {
		serviceConfig, err := loadServiceWithExtends(filename, name, servicesDict, workingDir, lookupEnv, opts, &cycleTracker{})
		if err != nil {
			return nil, err
		}

		services = append(services, *serviceConfig)
	}

	return services, nil
}

func loadServiceWithExtends(filename, name string, servicesDict map[string]interface{}, workingDir string, lookupEnv template.Mapping, opts *Options, ct *cycleTracker) (*types.ServiceConfig, error) {
	if err := ct.Add(filename, name); err != nil {
		return nil, err
	}

	target, ok := servicesDict[name]
	if !ok {
		return nil, fmt.Errorf("cannot extend service %q in %s: service not found", name, filename)
	}

	serviceConfig, err := LoadService(name, target.(map[string]interface{}), workingDir, lookupEnv, opts.ResolvePaths, opts.ConvertWindowsPaths)
	if err != nil {
		return nil, err
	}

	if serviceConfig.Extends != nil && !opts.SkipExtends {
		baseServiceName := *serviceConfig.Extends["service"]
		var baseService *types.ServiceConfig
		if file := serviceConfig.Extends["file"]; file == nil {
			baseService, err = loadServiceWithExtends(filename, baseServiceName, servicesDict, workingDir, lookupEnv, opts, ct)
			if err != nil {
				return nil, err
			}
		} else {
			// Resolve the path to the imported file, and load it.
			baseFilePath := absPath(workingDir, *file)

			bytes, err := ioutil.ReadFile(baseFilePath)
			if err != nil {
				return nil, err
			}

			baseFile, err := parseConfig(bytes, opts)
			if err != nil {
				return nil, err
			}

			baseFileServices := getSection(baseFile, "services")
			baseService, err = loadServiceWithExtends(baseFilePath, baseServiceName, baseFileServices, filepath.Dir(baseFilePath), lookupEnv, opts, ct)
			if err != nil {
				return nil, err
			}

			// Make paths relative to the importing Compose file. Note that we
			// make the paths relative to `*file` rather than `baseFilePath` so
			// that the resulting paths won't be absolute if `*file` isn't an
			// absolute path.
			baseFileParent := filepath.Dir(*file)
			if baseService.Build != nil {
				// Note that the Dockerfile is always defined relative to the
				// build context, so there's no need to update the Dockerfile field.
				baseService.Build.Context = absPath(baseFileParent, baseService.Build.Context)
			}

			for i, vol := range baseService.Volumes {
				if vol.Type != types.VolumeTypeBind {
					continue
				}
				baseService.Volumes[i].Source = absPath(baseFileParent, vol.Source)
			}
		}

		serviceConfig, err = _merge(baseService, serviceConfig)
		if err != nil {
			return nil, err
		}
	}

	return serviceConfig, nil
}

// LoadService produces a single ServiceConfig from a compose file Dict
// the serviceDict is not validated if directly used. Use Load() to enable validation
func LoadService(name string, serviceDict map[string]interface{}, workingDir string, lookupEnv template.Mapping, resolvePaths bool, convertPaths bool) (*types.ServiceConfig, error) {
	serviceConfig := &types.ServiceConfig{
		Scale: 1,
	}
	if err := Transform(serviceDict, serviceConfig); err != nil {
		return nil, err
	}
	serviceConfig.Name = name

	if err := resolveEnvironment(serviceConfig, workingDir, lookupEnv); err != nil {
		return nil, err
	}

	for i, volume := range serviceConfig.Volumes {
		if volume.Type != types.VolumeTypeBind {
			continue
		}

		if volume.Source == "" {
			return nil, errors.New(`invalid mount config for type "bind": field Source must not be empty`)
		}

		if resolvePaths {
			serviceConfig.Volumes[i] = resolveVolumePath(volume, workingDir, lookupEnv)
		}

		if convertPaths {
			serviceConfig.Volumes[i] = convertVolumePath(volume)
		}
	}

	return serviceConfig, nil
}

// Windows paths, c:\\my\\path\\shiny, need to be changed to be compatible with
// the Engine. Volume paths are expected to be linux style /c/my/path/shiny/
func convertVolumePath(volume types.ServiceVolumeConfig) types.ServiceVolumeConfig {
	volumeName := strings.ToLower(filepath.VolumeName(volume.Source))
	if len(volumeName) != 2 {
		return volume
	}

	convertedSource := fmt.Sprintf("/%c%s", volumeName[0], volume.Source[len(volumeName):])
	convertedSource = strings.ReplaceAll(convertedSource, "\\", "/")

	volume.Source = convertedSource
	return volume
}

func resolveEnvironment(serviceConfig *types.ServiceConfig, workingDir string, lookupEnv template.Mapping) error {
	environment := types.MappingWithEquals{}

	if len(serviceConfig.EnvFile) > 0 {
		for _, envFile := range serviceConfig.EnvFile {
			filePath := absPath(workingDir, envFile)
			file, err := os.Open(filePath)
			if err != nil {
				return err
			}
			defer file.Close()
			fileVars, err := dotenv.ParseWithLookup(file, dotenv.LookupFn(lookupEnv))
			if err != nil {
				return err
			}
			env := types.MappingWithEquals{}
			for k, v := range fileVars {
				v := v
				env[k] = &v
			}
			environment.OverrideBy(env.Resolve(lookupEnv).RemoveEmpty())
		}
	}

	environment.OverrideBy(serviceConfig.Environment.Resolve(lookupEnv))
	serviceConfig.Environment = environment
	return nil
}

func resolveVolumePath(volume types.ServiceVolumeConfig, workingDir string, lookupEnv template.Mapping) types.ServiceVolumeConfig {
	filePath := expandUser(volume.Source, lookupEnv)
	// Check if source is an absolute path (either Unix or Windows), to
	// handle a Windows client with a Unix daemon or vice-versa.
	//
	// Note that this is not required for Docker for Windows when specifying
	// a local Windows path, because Docker for Windows translates the Windows
	// path into a valid path within the VM.
	if !path.IsAbs(filePath) && !isAbs(filePath) {
		filePath = absPath(workingDir, filePath)
	}
	volume.Source = filePath
	return volume
}

// TODO: make this more robust
func expandUser(path string, lookupEnv template.Mapping) string {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			logrus.Warn("cannot expand '~', because the environment lacks HOME")
			return path
		}
		return filepath.Join(home, path[1:])
	}
	return path
}

func transformUlimits(data interface{}) (interface{}, error) {
	switch value := data.(type) {
	case int:
		return types.UlimitsConfig{Single: value}, nil
	case map[string]interface{}:
		ulimit := types.UlimitsConfig{}
		if v, ok := value["soft"]; ok {
			ulimit.Soft = v.(int)
		}
		if v, ok := value["hard"]; ok {
			ulimit.Hard = v.(int)
		}
		return ulimit, nil
	default:
		return data, errors.Errorf("invalid type %T for ulimits", value)
	}
}

// LoadNetworks produces a NetworkConfig map from a compose file Dict
// the source Dict is not validated if directly used. Use Load() to enable validation
func LoadNetworks(source map[string]interface{}) (map[string]types.NetworkConfig, error) {
	networks := make(map[string]types.NetworkConfig)
	err := Transform(source, &networks)
	if err != nil {
		return networks, err
	}
	for name, network := range networks {
		if !network.External.External {
			continue
		}
		switch {
		case network.External.Name != "":
			if network.Name != "" {
				return nil, errors.Errorf("network %s: network.external.name and network.name conflict; only use network.name", name)
			}
			logrus.Warnf("network %s: network.external.name is deprecated in favor of network.name", name)
			network.Name = network.External.Name
			network.External.Name = ""
		case network.Name == "":
			network.Name = name
		}
		networks[name] = network
	}
	return networks, nil
}

func externalVolumeError(volume, key string) error {
	return errors.Errorf(
		"conflicting parameters \"external\" and %q specified for volume %q",
		key, volume)
}

// LoadVolumes produces a VolumeConfig map from a compose file Dict
// the source Dict is not validated if directly used. Use Load() to enable validation
func LoadVolumes(source map[string]interface{}) (map[string]types.VolumeConfig, error) {
	volumes := make(map[string]types.VolumeConfig)
	if err := Transform(source, &volumes); err != nil {
		return volumes, err
	}

	for name, volume := range volumes {
		if !volume.External.External {
			continue
		}
		switch {
		case volume.Driver != "":
			return nil, externalVolumeError(name, "driver")
		case len(volume.DriverOpts) > 0:
			return nil, externalVolumeError(name, "driver_opts")
		case len(volume.Labels) > 0:
			return nil, externalVolumeError(name, "labels")
		case volume.External.Name != "":
			if volume.Name != "" {
				return nil, errors.Errorf("volume %s: volume.external.name and volume.name conflict; only use volume.name", name)
			}
			logrus.Warnf("volume %s: volume.external.name is deprecated in favor of volume.name", name)
			volume.Name = volume.External.Name
			volume.External.Name = ""
		case volume.Name == "":
			volume.Name = name
		}
		volumes[name] = volume
	}
	return volumes, nil
}

// LoadSecrets produces a SecretConfig map from a compose file Dict
// the source Dict is not validated if directly used. Use Load() to enable validation
func LoadSecrets(source map[string]interface{}, details types.ConfigDetails, resolvePaths bool) (map[string]types.SecretConfig, error) {
	secrets := make(map[string]types.SecretConfig)
	if err := Transform(source, &secrets); err != nil {
		return secrets, err
	}
	for name, secret := range secrets {
		obj, err := loadFileObjectConfig(name, "secret", types.FileObjectConfig(secret), details, resolvePaths)
		if err != nil {
			return nil, err
		}
		secretConfig := types.SecretConfig(obj)
		secrets[name] = secretConfig
	}
	return secrets, nil
}

// LoadConfigObjs produces a ConfigObjConfig map from a compose file Dict
// the source Dict is not validated if directly used. Use Load() to enable validation
func LoadConfigObjs(source map[string]interface{}, details types.ConfigDetails, resolvePaths bool) (map[string]types.ConfigObjConfig, error) {
	configs := make(map[string]types.ConfigObjConfig)
	if err := Transform(source, &configs); err != nil {
		return configs, err
	}
	for name, config := range configs {
		obj, err := loadFileObjectConfig(name, "config", types.FileObjectConfig(config), details, resolvePaths)
		if err != nil {
			return nil, err
		}
		configConfig := types.ConfigObjConfig(obj)
		configs[name] = configConfig
	}
	return configs, nil
}

func loadFileObjectConfig(name string, objType string, obj types.FileObjectConfig, details types.ConfigDetails, resolvePaths bool) (types.FileObjectConfig, error) {
	// if "external: true"
	switch {
	case obj.External.External:
		// handle deprecated external.name
		if obj.External.Name != "" {
			if obj.Name != "" {
				return obj, errors.Errorf("%[1]s %[2]s: %[1]s.external.name and %[1]s.name conflict; only use %[1]s.name", objType, name)
			}
			logrus.Warnf("%[1]s %[2]s: %[1]s.external.name is deprecated in favor of %[1]s.name", objType, name)
			obj.Name = obj.External.Name
			obj.External.Name = ""
		} else {
			if obj.Name == "" {
				obj.Name = name
			}
		}
		// if not "external: true"
	case obj.Driver != "":
		if obj.File != "" {
			return obj, errors.Errorf("%[1]s %[2]s: %[1]s.driver and %[1]s.file conflict; only use %[1]s.driver", objType, name)
		}
	default:
		if resolvePaths {
			obj.File = absPath(details.WorkingDir, obj.File)
		}
	}

	return obj, nil
}

func absPath(workingDir string, filePath string) string {
	if strings.HasPrefix(filePath, "~") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, filePath[1:])
	}
	if filepath.IsAbs(filePath) {
		return filePath
	}
	return filepath.Join(workingDir, filePath)
}

var transformMapStringString TransformerFunc = func(data interface{}) (interface{}, error) {
	switch value := data.(type) {
	case map[string]interface{}:
		return toMapStringString(value, false), nil
	case map[string]string:
		return value, nil
	default:
		return data, errors.Errorf("invalid type %T for map[string]string", value)
	}
}

var transformExternal TransformerFunc = func(data interface{}) (interface{}, error) {
	switch value := data.(type) {
	case bool:
		return map[string]interface{}{"external": value}, nil
	case map[string]interface{}:
		return map[string]interface{}{"external": true, "name": value["name"]}, nil
	default:
		return data, errors.Errorf("invalid type %T for external", value)
	}
}

var transformServicePort TransformerFunc = func(data interface{}) (interface{}, error) {
	switch entries := data.(type) {
	case []interface{}:
		// We process the list instead of individual items here.
		// The reason is that one entry might be mapped to multiple ServicePortConfig.
		// Therefore we take an input of a list and return an output of a list.
		var ports []interface{}
		for _, entry := range entries {
			switch value := entry.(type) {
			case int:
				parsed, err := types.ParsePortConfig(fmt.Sprint(value))
				if err != nil {
					return data, err
				}
				for _, v := range parsed {
					ports = append(ports, v)
				}
			case string:
				parsed, err := types.ParsePortConfig(value)
				if err != nil {
					return data, err
				}
				for _, v := range parsed {
					ports = append(ports, v)
				}
			case map[string]interface{}:
				published := value["published"]
				if v, ok := published.(int); ok {
					value["published"] = strconv.Itoa(v)
				}
				ports = append(ports, groupXFieldsIntoExtensions(value))
			default:
				return data, errors.Errorf("invalid type %T for port", value)
			}
		}
		return ports, nil
	default:
		return data, errors.Errorf("invalid type %T for port", entries)
	}
}

var transformServiceDeviceRequest TransformerFunc = func(data interface{}) (interface{}, error) {
	switch value := data.(type) {
	case map[string]interface{}:
		count, ok := value["count"]
		if ok {
			switch val := count.(type) {
			case int:
				return value, nil
			case string:
				if strings.ToLower(val) == "all" {
					value["count"] = -1
					return value, nil
				}
				return data, errors.Errorf("invalid string value for 'count' (the only value allowed is 'all')")
			default:
				return data, errors.Errorf("invalid type %T for device count", val)
			}
		}
		return data, nil
	default:
		return data, errors.Errorf("invalid type %T for resource reservation", value)
	}
}

var transformFileReferenceConfig TransformerFunc = func(data interface{}) (interface{}, error) {
	switch value := data.(type) {
	case string:
		return map[string]interface{}{"source": value}, nil
	case map[string]interface{}:
		if target, ok := value["target"]; ok {
			value["target"] = cleanTarget(target.(string))
		}
		return groupXFieldsIntoExtensions(value), nil
	default:
		return data, errors.Errorf("invalid type %T for secret", value)
	}
}

func cleanTarget(target string) string {
	if target == "" {
		return ""
	}
	return path.Clean(target)
}

var transformBuildConfig TransformerFunc = func(data interface{}) (interface{}, error) {
	switch value := data.(type) {
	case string:
		return map[string]interface{}{"context": value}, nil
	case map[string]interface{}:
		return groupXFieldsIntoExtensions(data.(map[string]interface{})), nil
	default:
		return data, errors.Errorf("invalid type %T for service build", value)
	}
}

var transformDependsOnConfig TransformerFunc = func(data interface{}) (interface{}, error) {
	switch value := data.(type) {
	case []interface{}:
		transformed := map[string]interface{}{}
		for _, serviceIntf := range value {
			service, ok := serviceIntf.(string)
			if !ok {
				return data, errors.Errorf("invalid type %T for service depends_on elementn, expected string", value)
			}
			transformed[service] = map[string]interface{}{"condition": types.ServiceConditionStarted}
		}
		return transformed, nil
	case map[string]interface{}:
		return groupXFieldsIntoExtensions(data.(map[string]interface{})), nil
	default:
		return data, errors.Errorf("invalid type %T for service depends_on", value)
	}
}

var transformExtendsConfig TransformerFunc = func(data interface{}) (interface{}, error) {
	switch data.(type) {
	case string:
		data = map[string]interface{}{
			"service": data,
		}
	}
	return transformMappingOrListFunc("=", true)(data)
}

var transformServiceVolumeConfig TransformerFunc = func(data interface{}) (interface{}, error) {
	switch value := data.(type) {
	case string:
		volume, err := ParseVolume(value)
		volume.Target = cleanTarget(volume.Target)
		return volume, err
	case map[string]interface{}:
		data := groupXFieldsIntoExtensions(data.(map[string]interface{}))
		if target, ok := data["target"]; ok {
			data["target"] = cleanTarget(target.(string))
		}
		return data, nil
	default:
		return data, errors.Errorf("invalid type %T for service volume", value)
	}
}

var transformServiceNetworkMap TransformerFunc = func(value interface{}) (interface{}, error) {
	if list, ok := value.([]interface{}); ok {
		mapValue := map[interface{}]interface{}{}
		for _, name := range list {
			mapValue[name] = nil
		}
		return mapValue, nil
	}
	return value, nil
}

var transformSSHConfig TransformerFunc = func(data interface{}) (interface{}, error) {
	switch value := data.(type) {
	case map[string]interface{}:
		var result []types.SSHKey
		for key, val := range value {
			if val == nil {
				val = ""
			}
			result = append(result, types.SSHKey{ID: key, Path: val.(string)})
		}
		return result, nil
	case []interface{}:
		var result []types.SSHKey
		for _, v := range value {
			key, val := transformValueToMapEntry(v.(string), "=", false)
			result = append(result, types.SSHKey{ID: key, Path: val.(string)})
		}
		return result, nil
	case string:
		return ParseShortSSHSyntax(value)
	}
	return nil, errors.Errorf("expected a sting, map or a list, got %T: %#v", data, data)
}

// ParseShortSSHSyntax parse short syntax for SSH authentications
func ParseShortSSHSyntax(value string) ([]types.SSHKey, error) {
	if value == "" {
		value = "default"
	}
	key, val := transformValueToMapEntry(value, "=", false)
	result := []types.SSHKey{{ID: key, Path: val.(string)}}
	return result, nil
}

var transformStringOrNumberList TransformerFunc = func(value interface{}) (interface{}, error) {
	list := value.([]interface{})
	result := make([]string, len(list))
	for i, item := range list {
		result[i] = fmt.Sprint(item)
	}
	return result, nil
}

var transformStringList TransformerFunc = func(data interface{}) (interface{}, error) {
	switch value := data.(type) {
	case string:
		return []string{value}, nil
	case []interface{}:
		return value, nil
	default:
		return data, errors.Errorf("invalid type %T for string list", value)
	}
}

func transformMappingOrListFunc(sep string, allowNil bool) TransformerFunc {
	return func(data interface{}) (interface{}, error) {
		return transformMappingOrList(data, sep, allowNil)
	}
}

func transformListOrMappingFunc(sep string, allowNil bool) TransformerFunc {
	return func(data interface{}) (interface{}, error) {
		return transformListOrMapping(data, sep, allowNil)
	}
}

func transformListOrMapping(listOrMapping interface{}, sep string, allowNil bool) (interface{}, error) {
	switch value := listOrMapping.(type) {
	case map[string]interface{}:
		return toStringList(value, sep, allowNil), nil
	case []interface{}:
		return listOrMapping, nil
	}
	return nil, errors.Errorf("expected a map or a list, got %T: %#v", listOrMapping, listOrMapping)
}

func transformMappingOrList(mappingOrList interface{}, sep string, allowNil bool) (interface{}, error) {
	switch value := mappingOrList.(type) {
	case map[string]interface{}:
		return toMapStringString(value, allowNil), nil
	case []interface{}:
		result := make(map[string]interface{})
		for _, value := range value {
			key, val := transformValueToMapEntry(value.(string), sep, allowNil)
			result[key] = val
		}
		return result, nil
	}
	return nil, errors.Errorf("expected a map or a list, got %T: %#v", mappingOrList, mappingOrList)
}

func transformValueToMapEntry(value string, separator string, allowNil bool) (string, interface{}) {
	parts := strings.SplitN(value, separator, 2)
	key := parts[0]
	switch {
	case len(parts) == 1 && allowNil:
		return key, nil
	case len(parts) == 1 && !allowNil:
		return key, ""
	default:
		return key, parts[1]
	}
}

var transformShellCommand TransformerFunc = func(value interface{}) (interface{}, error) {
	if str, ok := value.(string); ok {
		return shellwords.Parse(str)
	}
	return value, nil
}

var transformHealthCheckTest TransformerFunc = func(data interface{}) (interface{}, error) {
	switch value := data.(type) {
	case string:
		return append([]string{"CMD-SHELL"}, value), nil
	case []interface{}:
		return value, nil
	default:
		return value, errors.Errorf("invalid type %T for healthcheck.test", value)
	}
}

var transformSize TransformerFunc = func(value interface{}) (interface{}, error) {
	switch value := value.(type) {
	case int:
		return int64(value), nil
	case int64, types.UnitBytes:
		return value, nil
	case string:
		return units.RAMInBytes(value)
	default:
		return value, errors.Errorf("invalid type for size %T", value)
	}
}

var transformStringToDuration TransformerFunc = func(value interface{}) (interface{}, error) {
	switch value := value.(type) {
	case string:
		d, err := time.ParseDuration(value)
		if err != nil {
			return value, err
		}
		return types.Duration(d), nil
	case types.Duration:
		return value, nil
	default:
		return value, errors.Errorf("invalid type %T for duration", value)
	}
}

func toMapStringString(value map[string]interface{}, allowNil bool) map[string]interface{} {
	output := make(map[string]interface{})
	for key, value := range value {
		output[key] = toString(value, allowNil)
	}
	return output
}

func toString(value interface{}, allowNil bool) interface{} {
	switch {
	case value != nil:
		return fmt.Sprint(value)
	case allowNil:
		return nil
	default:
		return ""
	}
}

func toStringList(value map[string]interface{}, separator string, allowNil bool) []string {
	var output []string
	for key, value := range value {
		if value == nil && !allowNil {
			continue
		}
		output = append(output, fmt.Sprintf("%s%s%s", key, separator, value))
	}
	sort.Strings(output)
	return output
}
