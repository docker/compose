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
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"

	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/distribution/reference"
	"github.com/docker/buildx/util/imagetools"
	"github.com/docker/cli/cli/command"
	"github.com/docker/compose/v2/internal/ocipush"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/docker/compose/v2/pkg/prompt"
	"github.com/mikefarah/yq/v4/pkg/yqlib"
	"github.com/sirupsen/logrus"
	"gopkg.in/op/go-logging.v1"
)

// _backend redirects go-logging.v1 (used by yqlib) to logrus
type _backend struct{}

func (b *_backend) Log(l logging.Level, _ int, r *logging.Record) error {
	switch l {
	case logging.CRITICAL, logging.ERROR:
		logrus.Error(r.Message())
	case logging.WARNING:
		logrus.Warn(r.Message())
	case logging.NOTICE, logging.INFO:
		logrus.Info(r.Message())
	case logging.DEBUG:

		logrus.Debug(r.Message())
	}
	return nil
}

func init() {
	logging.SetBackend(&_backend{})
}

func (s *composeService) Publish(ctx context.Context, project *types.Project, repository string, options api.PublishOptions) error {
	return progress.RunWithTitle(ctx, func(ctx context.Context) error {
		return s.publish(ctx, project, repository, options)
	}, s.stdinfo(), "Publishing")
}

func (s *composeService) publish(ctx context.Context, project *types.Project, repository string, options api.PublishOptions) error {
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

	named, err := reference.ParseDockerRef(repository)
	if err != nil {
		return err
	}

	resolver := imagetools.New(imagetools.Opt{
		Auth: s.configFile(),
	})

	yqlib.InitExpressionParser()
	var layers []ocipush.Pushable
	extFiles := map[string]string{}
	for _, file := range project.ComposeFiles {
		data, err := processFile(ctx, file, project, extFiles)
		if err != nil {
			return err
		}

		layerDescriptor := ocipush.DescriptorForComposeFile(file, data)
		layers = append(layers, ocipush.Pushable{
			Descriptor: layerDescriptor,
			Data:       data,
		})
	}

	extLayers, err := processExtends(ctx, project, extFiles)
	if err != nil {
		return err
	}
	layers = append(layers, extLayers...)

	if options.WithEnvironment {
		layers = append(layers, envFileLayers(project)...)
	}

	if options.ResolveImageDigests {
		yaml, err := s.generateImageDigestsOverride(ctx, project)
		if err != nil {
			return err
		}

		layerDescriptor := ocipush.DescriptorForComposeFile("image-digests.yaml", yaml)
		layers = append(layers, ocipush.Pushable{
			Descriptor: layerDescriptor,
			Data:       yaml,
		})
	}

	w := progress.ContextWriter(ctx)
	w.Event(progress.Event{
		ID:     repository,
		Text:   "publishing",
		Status: progress.Working,
	})
	if !s.dryRun {
		err = ocipush.PushManifest(ctx, resolver, named, layers, options.OCIVersion)
		if err != nil {
			w.Event(progress.Event{
				ID:     repository,
				Text:   "publishing",
				Status: progress.Error,
			})
			return err
		}
	}
	w.Event(progress.Event{
		ID:     repository,
		Text:   "published",
		Status: progress.Done,
	})
	return nil
}

func processExtends(ctx context.Context, project *types.Project, extFiles map[string]string) ([]ocipush.Pushable, error) {
	var layers []ocipush.Pushable
	moreExtFiles := map[string]string{}
	for xf, hash := range extFiles {
		data, err := processFile(ctx, xf, project, moreExtFiles)
		if err != nil {
			return nil, err
		}

		layerDescriptor := ocipush.DescriptorForComposeFile(hash, data)
		layerDescriptor.Annotations["com.docker.compose.extends"] = "true"
		layers = append(layers, ocipush.Pushable{
			Descriptor: layerDescriptor,
			Data:       data,
		})
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

func processFile(ctx context.Context, file string, project *types.Project, extFiles map[string]string) ([]byte, error) {
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
	})
	if err != nil {
		return nil, err
	}
	for name, service := range base.Services {
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

		// FIXME implement without relying on yqlib so we don't get extra dependencies
		exp := fmt.Sprintf(".services.%s.extends.file = %q", name, hash)
		encoder := yqlib.NewYamlEncoder(yqlib.YamlPreferences{
			ColorsEnabled: false,
		})
		decoder := yqlib.NewYamlDecoder(yqlib.YamlPreferences{})
		out, err := yqlib.NewStringEvaluator().Evaluate(exp, string(f), encoder, decoder)
		if err != nil {
			return nil, err
		}

		f = []byte(out)
	}
	return f, nil
}

func (s *composeService) generateImageDigestsOverride(ctx context.Context, project *types.Project) ([]byte, error) {
	project, err := project.WithProfiles([]string{"*"})
	if err != nil {
		return nil, err
	}
	project, err = project.WithImagesResolved(ImageDigestResolver(ctx, s.configFile(), s.apiClient()))
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

func (s *composeService) preChecks(project *types.Project, options api.PublishOptions) (bool, error) {
	if ok, err := s.checkOnlyBuildSection(project); !ok {
		return false, err
	}
	envVariables, err := s.checkEnvironmentVariables(project, options)
	if err != nil {
		return false, err
	}
	if !options.AssumeYes && len(envVariables) > 0 {
		fmt.Println("you are about to publish environment variables within your OCI artifact.\n" +
			"please double check that you are not leaking sensitive data")
		for key, val := range envVariables {
			_, _ = fmt.Fprintln(s.dockerCli.Out(), "Service/Config ", key)
			for k, v := range val {
				_, _ = fmt.Fprintf(s.dockerCli.Out(), "%s=%v\n", k, *v)
			}
		}
		return acceptPublishEnvVariables(s.dockerCli)
	}

	for name, config := range project.Services {
		for _, volume := range config.Volumes {
			if volume.Type == types.VolumeTypeBind {
				return false, fmt.Errorf("cannot publish compose file: service %q relies on bind-mount. You should use volumes", name)
			}
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

func acceptPublishEnvVariables(cli command.Cli) (bool, error) {
	msg := "Are you ok to publish these environment variables? [y/N]: "
	confirm, err := prompt.NewPrompt(cli.In(), cli.Out()).Confirm(msg, false)
	return confirm, err
}

func envFileLayers(project *types.Project) []ocipush.Pushable {
	var layers []ocipush.Pushable
	for _, service := range project.Services {
		for _, envFile := range service.EnvFiles {
			f, err := os.ReadFile(envFile.Path)
			if err != nil {
				// if we can't read the file, skip to the next one
				continue
			}
			layerDescriptor := ocipush.DescriptorForEnvFile(envFile.Path, f)
			layers = append(layers, ocipush.Pushable{
				Descriptor: layerDescriptor,
				Data:       f,
			})
		}
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
