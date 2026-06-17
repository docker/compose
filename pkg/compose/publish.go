/*
   Copyright 2020 Docker Compose CLI authors

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

package compose

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/DefangLabs/secret-detector/pkg/detectors/keyword"
	"github.com/DefangLabs/secret-detector/pkg/scanner"
	"github.com/DefangLabs/secret-detector/pkg/secrets"
	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/distribution/reference"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
	"go.yaml.in/yaml/v4"

	"github.com/docker/compose/v5/internal/desktop"
	"github.com/docker/compose/v5/internal/oci"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/compose/transform"
)

func (s *composeService) Publish(ctx context.Context, project *types.Project, repository string, options api.PublishOptions) error {
	return Run(ctx, func(ctx context.Context) error {
		return s.publish(ctx, project, repository, options)
	}, "publish", s.events)
}

//nolint:gocyclo
func (s *composeService) publish(ctx context.Context, project *types.Project, repository string, options api.PublishOptions) error {
	project, err := project.WithProfiles([]string{"*"})
	if err != nil {
		return err
	}
	accept, err := s.preChecks(ctx, project, options)
	if err != nil {
		return err
	}
	if !accept {
		return api.ErrCanceled
	}
	err = s.Push(ctx, project, api.PushOptions{IgnoreFailures: true, ImageMandatory: true})
	if err != nil {
		return err
	}

	layers, err := s.createLayers(ctx, project, options)
	if err != nil {
		return err
	}

	s.events.On(api.Resource{
		ID:     repository,
		Text:   "publishing",
		Status: api.Working,
	})
	if logrus.IsLevelEnabled(logrus.DebugLevel) {
		logrus.Debug("publishing layers")
		for _, layer := range layers {
			indent, _ := json.MarshalIndent(layer, "", "  ")
			fmt.Println(string(indent))
		}
	}
	if !s.dryRun {
		named, err := reference.ParseDockerRef(repository)
		if err != nil {
			return err
		}

		var insecureRegistries []string
		if options.InsecureRegistry {
			insecureRegistries = append(insecureRegistries, reference.Domain(named))
		}

		resolver := oci.NewResolver(s.configFile(), desktop.ProxyTransportFor(ctx, s.apiClient()), insecureRegistries...)

		descriptor, err := oci.PushManifest(ctx, resolver, named, layers, options.OCIVersion)
		if err != nil {
			s.events.On(api.Resource{
				ID:     repository,
				Text:   "publishing",
				Status: api.Error,
			})
			return err
		}

		if options.Application {
			manifests := []v1.Descriptor{}
			for _, service := range project.Services {
				ref, err := reference.ParseDockerRef(service.Image)
				if err != nil {
					return err
				}

				manifest, err := oci.Copy(ctx, resolver, ref, named)
				if err != nil {
					return err
				}
				manifests = append(manifests, manifest)
			}

			descriptor.Data = nil
			index, err := json.Marshal(v1.Index{
				Versioned: specs.Versioned{SchemaVersion: 2},
				MediaType: v1.MediaTypeImageIndex,
				Manifests: manifests,
				Subject:   &descriptor,
				Annotations: map[string]string{
					"com.docker.compose.version": api.ComposeVersion,
				},
			})
			if err != nil {
				return err
			}
			imagesDescriptor := v1.Descriptor{
				MediaType:    v1.MediaTypeImageIndex,
				ArtifactType: oci.ComposeProjectArtifactType,
				Digest:       digest.FromString(string(index)),
				Size:         int64(len(index)),
				Annotations: map[string]string{
					"com.docker.compose.version": api.ComposeVersion,
				},
				Data: index,
			}
			err = oci.Push(ctx, resolver, reference.TrimNamed(named), imagesDescriptor)
			if err != nil {
				return err
			}
		}
	}
	s.events.On(api.Resource{
		ID:     repository,
		Text:   "published",
		Status: api.Done,
	})
	return nil
}

func (s *composeService) createLayers(ctx context.Context, project *types.Project, options api.PublishOptions) ([]v1.Descriptor, error) {
	var layers []v1.Descriptor
	extFiles := map[string]string{}
	envFiles := map[string]string{}
	for _, file := range project.ComposeFiles {
		data, err := processFile(ctx, file, project, extFiles, envFiles)
		if err != nil {
			return nil, err
		}

		layerDescriptor := oci.DescriptorForComposeFile(file, data)
		layers = append(layers, layerDescriptor)
	}

	extLayers, err := processExtends(ctx, project, extFiles)
	if err != nil {
		return nil, err
	}
	layers = append(layers, extLayers...)

	if options.WithEnvironment {
		layers = append(layers, envFileLayers(envFiles)...)
	}

	if options.ResolveImageDigests {
		yaml, err := s.generateImageDigestsOverride(ctx, project)
		if err != nil {
			return nil, err
		}

		layerDescriptor := oci.DescriptorForComposeFile("image-digests.yaml", yaml)
		layers = append(layers, layerDescriptor)
	}
	return layers, nil
}

func processExtends(ctx context.Context, project *types.Project, extFiles map[string]string) ([]v1.Descriptor, error) {
	var layers []v1.Descriptor
	moreExtFiles := map[string]string{}
	envFiles := map[string]string{}
	for xf, hash := range extFiles {
		data, err := processFile(ctx, xf, project, moreExtFiles, envFiles)
		if err != nil {
			return nil, err
		}

		layerDescriptor := oci.DescriptorForComposeFile(hash, data)
		layerDescriptor.Annotations["com.docker.compose.extends"] = "true"
		layers = append(layers, layerDescriptor)
	}
	for f, hash := range moreExtFiles {
		if _, ok := extFiles[f]; ok {
			delete(moreExtFiles, f)
		}
		extFiles[f] = hash
	}
	if len(moreExtFiles) > 0 {
		extLayers, err := processExtends(ctx, project, moreExtFiles)
		if err != nil {
			return nil, err
		}
		layers = append(layers, extLayers...)
	}
	return layers, nil
}

func processFile(ctx context.Context, file string, project *types.Project, extFiles map[string]string, envFiles map[string]string) ([]byte, error) {
	f, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}

	base, err := loader.LoadWithContext(ctx, types.ConfigDetails{
		WorkingDir:  project.WorkingDir,
		Environment: project.Environment,
		ConfigFiles: []types.ConfigFile{
			{
				Filename: file,
				Content:  f,
			},
		},
	}, func(options *loader.Options) {
		options.SkipValidation = true
		options.SkipExtends = true
		options.SkipConsistencyCheck = true
		options.ResolvePaths = true
		options.SkipInclude = true
		options.Profiles = project.Profiles
	})
	if err != nil {
		return nil, err
	}
	for name, service := range base.Services {
		for i, envFile := range service.EnvFiles {
			hash := fmt.Sprintf("%x.env", sha256.Sum256([]byte(envFile.Path)))
			envFiles[envFile.Path] = hash
			f, err = transform.ReplaceEnvFile(f, name, i, hash)
			if err != nil {
				return nil, err
			}
		}

		if service.Extends == nil {
			continue
		}
		xf := service.Extends.File
		if xf == "" {
			continue
		}
		if _, err = os.Stat(service.Extends.File); os.IsNotExist(err) {
			// No local file, while we loaded the project successfully: This is actually a remote resource
			continue
		}

		hash := fmt.Sprintf("%x.yaml", sha256.Sum256([]byte(xf)))
		extFiles[xf] = hash

		f, err = transform.ReplaceExtendsFile(f, name, hash)
		if err != nil {
			return nil, err
		}
	}
	return f, nil
}

func (s *composeService) generateImageDigestsOverride(ctx context.Context, project *types.Project) ([]byte, error) {
	project, err := project.WithImagesResolved(ImageDigestResolver(ctx, s.configFile(), s.apiClient()))
	if err != nil {
		return nil, err
	}
	override := types.Project{
		Services: types.Services{},
	}
	for name, service := range project.Services {
		override.Services[name] = types.ServiceConfig{
			Image: service.Image,
		}
	}
	return override.MarshalYAML()
}

func (s *composeService) preChecks(ctx context.Context, project *types.Project, options api.PublishOptions) (bool, error) {
	if ok, err := s.checkOnlyBuildSection(project); !ok || err != nil {
		return false, err
	}
	bindMounts := s.checkForBindMount(project)
	if len(bindMounts) > 0 {
		b := strings.Builder{}
		b.WriteString("you are about to publish bind mounts declaration within your OCI artifact.\n" +
			"only the bind mount declarations will be added to the OCI artifact (not content)\n" +
			"please double check that you are not mounting potential user's sensitive directories or data\n")
		for key, val := range bindMounts {
			b.WriteString(key)
			for _, v := range val {
				b.WriteString(v.String())
				b.WriteRune('\n')
			}
		}
		b.WriteString("Are you ok to publish these bind mount declarations?")
		confirm, err := s.prompt(b.String(), false)
		if err != nil || !confirm {
			return false, err
		}
	}
	detectedSecrets, err := s.checkForSensitiveData(ctx, project)
	if err != nil {
		return false, err
	}
	if len(detectedSecrets) > 0 {
		b := strings.Builder{}
		b.WriteString("you are about to publish sensitive data within your OCI artifact.\n" +
			"please double check that you are not leaking sensitive data\n")
		for _, val := range detectedSecrets {
			b.WriteString(val.Type)
			b.WriteRune('\n')
			fmt.Fprintf(&b, "%q: %s\n", val.Key, val.Value)
		}
		b.WriteString("Are you ok to publish these sensitive data?")
		confirm, err := s.prompt(b.String(), false)
		if err != nil || !confirm {
			return false, err
		}
	}
	err = s.checkEnvironmentVariables(ctx, project, options)
	if err != nil {
		return false, err
	}
	return true, nil
}

// envCheckFindings groups everything checkEnvironmentVariables surfaces to
// the user during publish pre-checks for env-related leak risks.
type envCheckFindings struct {
	// services maps service name -> findings for that service. Only services
	// with at least one finding are present.
	services map[string]*serviceEnvFindings
	// configsLiteralContent lists configs whose inline `content:` is a literal
	// (not interpolation). Sorted alphabetically. config.content is decoupled
	// from --with-env because the flag is documented as controlling environment
	// variable publishing only.
	configsLiteralContent []string
}

type serviceEnvFindings struct {
	hasEnvFile bool
	// suspiciousKeys is the set of environment variable names whose literal
	// values look sensitive, as classified by the upstream DefangLabs keyword
	// detector (password, secret, token, api_key, …). A set is used because
	// the same service may be visited across multiple compose files during
	// the extends walk; callers convert to a sorted slice via sortedKeys
	// when surfacing to the user.
	suspiciousKeys map[string]struct{}
}

// sortedSuspiciousKeys returns the suspicious env var names alphabetically
// sorted for stable output.
func (f *serviceEnvFindings) sortedSuspiciousKeys() []string {
	return sortedMapKeys(f.suspiciousKeys)
}

func sortedMapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func (f *envCheckFindings) hasEnvFinding() bool {
	for _, svc := range f.services {
		if svc.hasEnvFile || len(svc.suspiciousKeys) > 0 {
			return true
		}
	}
	return false
}

// checkEnvironmentVariables walks every compose file that will be serialized
// into the OCI artifact (the top-level files plus any local extends parents)
// and prompts the user to confirm before publishing:
//
//  1. service env_file declarations and literal environment values whose key
//     name looks sensitive (password, secret, token, api_key, …) — silenced
//     by --with-env;
//  2. literal inline config.content — always prompts (decoupled from
//     --with-env, which is documented to cover env vars only).
//
// Interpolated values like "${SECRET}" or "$VAR" are preserved as placeholders
// in the published YAML and don't leak the resolved value; the keyword
// detector's value regex skips them automatically.
func (s *composeService) checkEnvironmentVariables(ctx context.Context, project *types.Project, options api.PublishOptions) error {
	if len(project.ComposeFiles) == 0 {
		return nil
	}

	findings, err := collectEnvCheckFindings(ctx, project)
	if err != nil {
		return err
	}

	if !options.WithEnvironment && findings.hasEnvFinding() {
		if err := s.confirmOrCancel(buildEnvPromptMessage(findings.services)); err != nil {
			return err
		}
	}

	if len(findings.configsLiteralContent) > 0 {
		if err := s.confirmOrCancel(buildConfigContentPromptMessage(findings.configsLiteralContent)); err != nil {
			return err
		}
	}

	return nil
}

// confirmOrCancel runs an interactive yes/no prompt and returns:
//   - the prompt's error verbatim, if it failed;
//   - api.ErrCanceled if the user declined;
//   - nil if the user accepted.
func (s *composeService) confirmOrCancel(message string) error {
	confirm, err := s.prompt(message, false)
	if err != nil {
		return err
	}
	if !confirm {
		return api.ErrCanceled
	}
	return nil
}

// collectEnvCheckFindings walks every compose file scheduled for publication
// (top-level files plus any local extends parents discovered along the way)
// and aggregates per-service and per-config findings. The walk mirrors
// processExtends so coverage matches what is actually serialized into the OCI
// artifact.
func collectEnvCheckFindings(ctx context.Context, project *types.Project) (*envCheckFindings, error) {
	findings := &envCheckFindings{services: map[string]*serviceEnvFindings{}}
	literalCfgs := map[string]struct{}{}
	keywordDetector := keyword.NewDetector("0")

	seen := map[string]struct{}{}
	queue := slices.Clone(project.ComposeFiles)
	for len(queue) > 0 {
		file := queue[0]
		queue = queue[1:]
		if _, ok := seen[file]; ok {
			continue
		}
		seen[file] = struct{}{}

		unresolved, err := loadUnresolvedFile(ctx, project, file)
		if err != nil {
			return nil, fmt.Errorf("failed to load compose file %s: %w", file, err)
		}

		for _, service := range unresolved.Services {
			recordServiceEnvFindings(findings.services, keywordDetector, service)
			if parent := localExtendsParent(service); parent != "" {
				queue = append(queue, parent)
			}
		}
		for name, config := range unresolved.Configs {
			// config.Environment is a variable *name* (only the name is
			// published, not its resolved value) so it is not a leak. Inline
			// config.Content is what ends up in the artifact. compose-go
			// enforces that file, environment, and content are mutually
			// exclusive. The map key is the name as written in the compose
			// file; config.Name is the project-namespaced version, which is
			// less helpful when surfaced to the user.
			if config.Content != "" && configContentLooksLiteral(config.Content, keywordDetector) {
				literalCfgs[name] = struct{}{}
			}
		}
	}

	if len(literalCfgs) > 0 {
		findings.configsLiteralContent = sortedMapKeys(literalCfgs)
	}
	return findings, nil
}

func recordServiceEnvFindings(services map[string]*serviceEnvFindings, detector secrets.Detector, service types.ServiceConfig) {
	envValues := map[string]string{}
	for key, value := range service.Environment {
		if value == nil {
			continue
		}
		envValues[key] = replaceDollarEscape(*value)
	}

	hits, _ := detector.ScanMap(envValues)
	if len(hits) == 0 && len(service.EnvFiles) == 0 {
		return
	}

	f := services[service.Name]
	if f == nil {
		f = &serviceEnvFindings{suspiciousKeys: map[string]struct{}{}}
		services[service.Name] = f
	}
	if len(service.EnvFiles) > 0 {
		f.hasEnvFile = true
	}
	for _, hit := range hits {
		f.suspiciousKeys[hit.Key] = struct{}{}
	}
}

// configContentLooksLiteral returns true when the inline config.content has
// a literal portion that would be published as-is, leaking the value to
// consumers of the OCI artifact.
//
// We piggyback on the keyword detector's value regex (`[^${\s].+[^}\s]`) by
// passing a fake "password" key to ScanMap — the regex isn't exported
// directly, only via the key+value match path. The regex excludes values
// starting with `$` (`${VAR}`/`$VAR` interpolation), ending with `}`
// (templates like `key=${SECRET}`), or shorter than 3 chars, which neatly
// matches our notion of "looks like a template, not a literal".
func configContentLooksLiteral(content string, detector secrets.Detector) bool {
	hits, _ := detector.ScanMap(map[string]string{"password": replaceDollarEscape(content)})
	return len(hits) > 0
}

// replaceDollarEscape substitutes the compose-spec `$$` escape (which
// represents a literal `$` in the resolved value) with a placeholder. The
// placeholder is `X` rather than `$` because the keyword detector's value
// regex excludes any value beginning with `$`; using `$` would mask the
// literal we're trying to flag. Any non-special char would do — we picked
// `X` for readability.
func replaceDollarEscape(value string) string {
	return strings.ReplaceAll(value, "$$", "X")
}

// localExtendsParent returns the path of an extends parent file that exists on
// disk, or "" when the service does not extend or extends a remote resource.
func localExtendsParent(service types.ServiceConfig) string {
	if service.Extends == nil || service.Extends.File == "" {
		return ""
	}
	if _, err := os.Stat(service.Extends.File); err != nil {
		return ""
	}
	return service.Extends.File
}

func buildEnvPromptMessage(services map[string]*serviceEnvFindings) string {
	var b strings.Builder
	b.WriteString("you are about to publish env-related declarations within your OCI artifact.\n")
	b.WriteString("env_file paths and literal values for sensitive-looking keys are embedded as-is in the published YAML;\n")
	b.WriteString("interpolated values like \"${VAR}\" are kept symbolic and have already been excluded.\n")
	for _, name := range sortedMapKeys(services) {
		f := services[name]
		if f.hasEnvFile {
			fmt.Fprintf(&b, "  service %q: env_file declared\n", name)
		}
		if keys := f.sortedSuspiciousKeys(); len(keys) > 0 {
			quoted := make([]string, len(keys))
			for i, k := range keys {
				quoted[i] = fmt.Sprintf("%q", k)
			}
			fmt.Fprintf(&b, "  service %q: literal value for %s\n", name, strings.Join(quoted, ", "))
		}
	}
	b.WriteString("Use --with-env to silence this prompt and always publish env declarations.\n")
	b.WriteString("Are you ok to publish these env declarations?")
	return b.String()
}

func buildConfigContentPromptMessage(configs []string) string {
	var b strings.Builder
	b.WriteString("you are about to publish literal inline config content within your OCI artifact.\n")
	for _, name := range configs {
		fmt.Fprintf(&b, "  config %q\n", name)
	}
	b.WriteString("Are you ok to publish these config contents?")
	return b.String()
}

// loadUnresolvedFile loads a single compose file with interpolation and
// environment resolution skipped, so callers can inspect raw user-provided
// values. Used by both checkEnvironmentVariables and composeFileAsByteReader.
func loadUnresolvedFile(ctx context.Context, project *types.Project, filePath string) (*types.Project, error) {
	return loader.LoadWithContext(ctx, types.ConfigDetails{
		WorkingDir:  project.WorkingDir,
		Environment: project.Environment,
		ConfigFiles: []types.ConfigFile{{Filename: filePath}},
	}, func(options *loader.Options) {
		options.SkipValidation = true
		options.SkipExtends = true
		options.SkipConsistencyCheck = true
		options.ResolvePaths = true
		// SkipInclude mirrors processFile: include directives stay symbolic in
		// the published artifact, so included content must not be inspected
		// here either (otherwise we'd flag literals that never ship).
		options.SkipInclude = true
		options.SkipInterpolation = true
		options.SkipResolveEnvironment = true
		options.Profiles = project.Profiles
	})
}

func envFileLayers(files map[string]string) []v1.Descriptor {
	var layers []v1.Descriptor
	for file, hash := range files {
		f, err := os.ReadFile(file)
		if err != nil {
			// if we can't read the file, skip to the next one
			continue
		}
		layerDescriptor := oci.DescriptorForEnvFile(hash, f)
		layers = append(layers, layerDescriptor)
	}
	return layers
}

func (s *composeService) checkOnlyBuildSection(project *types.Project) (bool, error) {
	errorList := []string{}
	for _, service := range project.Services {
		if service.Image == "" && service.Build != nil {
			errorList = append(errorList, service.Name)
		}
	}
	if len(errorList) > 0 {
		var errMsg strings.Builder
		errMsg.WriteString("your Compose stack cannot be published as it only contains a build section for service(s):\n")
		for _, serviceInError := range errorList {
			fmt.Fprintf(&errMsg, "- %q\n", serviceInError)
		}
		return false, errors.New(errMsg.String())
	}
	return true, nil
}

func (s *composeService) checkForBindMount(project *types.Project) map[string][]types.ServiceVolumeConfig {
	allFindings := map[string][]types.ServiceVolumeConfig{}
	for serviceName, config := range project.Services {
		bindMounts := []types.ServiceVolumeConfig{}
		for _, volume := range config.Volumes {
			if volume.Type == types.VolumeTypeBind {
				bindMounts = append(bindMounts, volume)
			}
		}
		if len(bindMounts) > 0 {
			allFindings[serviceName] = bindMounts
		}
	}
	return allFindings
}

func (s *composeService) checkForSensitiveData(ctx context.Context, project *types.Project) ([]secrets.DetectedSecret, error) {
	var allFindings []secrets.DetectedSecret
	scan := scanner.NewDefaultScanner()
	// Check all compose files
	for _, file := range project.ComposeFiles {
		in, err := composeFileAsByteReader(ctx, file, project)
		if err != nil {
			return nil, err
		}

		findings, err := scan.ScanReader(in)
		if err != nil {
			return nil, fmt.Errorf("failed to scan compose file %s: %w", file, err)
		}
		allFindings = append(allFindings, findings...)
	}
	for _, service := range project.Services {
		// Check env files
		for _, envFile := range service.EnvFiles {
			findings, err := scan.ScanFile(envFile.Path)
			if err != nil {
				return nil, fmt.Errorf("failed to scan env file %s: %w", envFile.Path, err)
			}
			allFindings = append(allFindings, findings...)
		}
	}

	// Check configs defined by files
	for _, config := range project.Configs {
		if config.File != "" {
			findings, err := scan.ScanFile(config.File)
			if err != nil {
				return nil, fmt.Errorf("failed to scan config file %s: %w", config.File, err)
			}
			allFindings = append(allFindings, findings...)
		}
	}

	// Check secrets defined by files
	for _, secret := range project.Secrets {
		if secret.File != "" {
			findings, err := scan.ScanFile(secret.File)
			if err != nil {
				return nil, fmt.Errorf("failed to scan secret file %s: %w", secret.File, err)
			}
			allFindings = append(allFindings, findings...)
		}
	}

	return allFindings, nil
}

func composeFileAsByteReader(ctx context.Context, filePath string, project *types.Project) (io.Reader, error) {
	model, err := loader.LoadModelWithContext(ctx, types.ConfigDetails{
		WorkingDir:  project.WorkingDir,
		Environment: project.Environment,
		ConfigFiles: []types.ConfigFile{{Filename: filePath}},
	}, func(options *loader.Options) {
		options.SkipValidation = true
		options.SkipExtends = true
		options.SkipConsistencyCheck = true
		options.ResolvePaths = true
		options.SkipInclude = true
		options.SkipInterpolation = true
		options.SkipResolveEnvironment = true
		options.Profiles = project.Profiles
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load compose file %s: %w", filePath, err)
	}
	in, err := yaml.Marshal(model)
	if err != nil {
		return nil, err
	}
	return bytes.NewBuffer(in), nil
}
