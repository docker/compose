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
	"strings"

	"github.com/DefangLabs/secret-detector/pkg/scanner"
	"github.com/DefangLabs/secret-detector/pkg/secrets"
	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/distribution/reference"
	"github.com/docker/compose/v5/internal/oci"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/compose/transform"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
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
	accept, err := s.preChecks(project, options)
	if err != nil {
		return err
	}
	if !accept {
		return nil
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

		resolver := oci.NewResolver(s.configFile(), insecureRegistries...)

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
	for xf, hash := range extFiles {
		data, err := processFile(ctx, xf, project, moreExtFiles, nil)
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

//nolint:gocyclo
func (s *composeService) preChecks(project *types.Project, options api.PublishOptions) (bool, error) {
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
	detectedSecrets, err := s.checkForSensitiveData(project)
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
			b.WriteString(fmt.Sprintf("%q: %s\n", val.Key, val.Value))
		}
		b.WriteString("Are you ok to publish these sensitive data?")
		confirm, err := s.prompt(b.String(), false)
		if err != nil || !confirm {
			return false, err
		}
	}
	envVariables, err := s.checkEnvironmentVariables(project, options)
	if err != nil {
		return false, err
	}
	if len(envVariables) > 0 {
		b := strings.Builder{}
		b.WriteString("you are about to publish environment variables within your OCI artifact.\n" +
			"please double check that you are not leaking sensitive data\n")
		for key, val := range envVariables {
			b.WriteString("Service/Config  ")
			b.WriteString(key)
			b.WriteRune('\n')
			for k, v := range val {
				b.WriteString(fmt.Sprintf("%s=%v\n", k, *v))
			}
		}
		b.WriteString("Are you ok to publish these environment variables?")
		confirm, err := s.prompt(b.String(), false)
		if err != nil || !confirm {
			return false, err
		}
	}
	return true, nil
}

func (s *composeService) checkEnvironmentVariables(project *types.Project, options api.PublishOptions) (map[string]types.MappingWithEquals, error) {
	envVarList := map[string]types.MappingWithEquals{}
	errorList := map[string][]string{}

	for _, service := range project.Services {
		if len(service.EnvFiles) > 0 {
			errorList[service.Name] = append(errorList[service.Name], fmt.Sprintf("service %q has env_file declared.", service.Name))
		}
		if len(service.Environment) > 0 {
			errorList[service.Name] = append(errorList[service.Name], fmt.Sprintf("service %q has environment variable(s) declared.", service.Name))
			envVarList[service.Name] = service.Environment
		}
	}

	for _, config := range project.Configs {
		if config.Environment != "" {
			errorList[config.Name] = append(errorList[config.Name], fmt.Sprintf("config %q is declare as an environment variable.", config.Name))
			envVarList[config.Name] = types.NewMappingWithEquals([]string{fmt.Sprintf("%s=%s", config.Name, config.Environment)})
		}
	}

	if !options.WithEnvironment && len(errorList) > 0 {
		errorMsgSuffix := "To avoid leaking sensitive data, you must either explicitly allow the sending of environment variables by using the --with-env flag,\n" +
			"or remove sensitive data from your Compose configuration"
		errorMsg := ""
		for _, errors := range errorList {
			for _, err := range errors {
				errorMsg += fmt.Sprintf("%s\n", err)
			}
		}
		return nil, fmt.Errorf("%s%s", errorMsg, errorMsgSuffix)

	}
	return envVarList, nil
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
		errMsg := "your Compose stack cannot be published as it only contains a build section for service(s):\n"
		for _, serviceInError := range errorList {
			errMsg += fmt.Sprintf("- %q\n", serviceInError)
		}
		return false, errors.New(errMsg)
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

func (s *composeService) checkForSensitiveData(project *types.Project) ([]secrets.DetectedSecret, error) {
	var allFindings []secrets.DetectedSecret
	scan := scanner.NewDefaultScanner()
	// Check all compose files
	for _, file := range project.ComposeFiles {
		in, err := composeFileAsByteReader(file, project)
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

func composeFileAsByteReader(filePath string, project *types.Project) (io.Reader, error) {
	composeFile, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open compose file %s: %w", filePath, err)
	}
	base, err := loader.LoadWithContext(context.TODO(), types.ConfigDetails{
		WorkingDir:  project.WorkingDir,
		Environment: project.Environment,
		ConfigFiles: []types.ConfigFile{
			{
				Filename: filePath,
				Content:  composeFile,
			},
		},
	}, func(options *loader.Options) {
		options.SkipValidation = true
		options.SkipExtends = true
		options.SkipConsistencyCheck = true
		options.ResolvePaths = true
		options.SkipInterpolation = true
		options.SkipResolveEnvironment = true
	})
	if err != nil {
		return nil, err
	}

	in, err := base.MarshalYAML()
	if err != nil {
		return nil, err
	}
	return bytes.NewBuffer(in), nil
}
