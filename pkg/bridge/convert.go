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

package bridge

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strconv"

	"github.com/compose-spec/compose-go/v2/types"
	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/cli/cli/command"
	cli "github.com/docker/cli/cli/command/container"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/utils"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/go-connections/nat"
	"gopkg.in/yaml.v3"
)

type ConvertOptions struct {
	Output          string
	Templates       string
	Transformations []string
}

func Convert(ctx context.Context, dockerCli command.Cli, project *types.Project, opts ConvertOptions) error {
	if len(opts.Transformations) == 0 {
		opts.Transformations = []string{DefaultTransformerImage}
	}
	// Load image references, secrets and configs, also expose ports
	project, err := LoadAdditionalResources(ctx, dockerCli, project)
	if err != nil {
		return err
	}
	// for user to rely on compose.yaml attribute names, not go struct ones, we marshall back into YAML
	raw, err := project.MarshalYAML(types.WithSecretContent)
	// Marshall to YAML
	if err != nil {
		return fmt.Errorf("cannot render project into yaml: %w", err)
	}
	var model map[string]any
	err = yaml.Unmarshal(raw, &model)
	if err != nil {
		return fmt.Errorf("cannot render project into yaml: %w", err)
	}

	if opts.Output != "" {
		_ = os.RemoveAll(opts.Output)
		err := os.MkdirAll(opts.Output, 0o744)
		if err != nil && !os.IsExist(err) {
			return fmt.Errorf("cannot create output folder: %w", err)
		}
	}
	// Run Transformers images
	return convert(ctx, dockerCli, model, opts)
}

func convert(ctx context.Context, dockerCli command.Cli, model map[string]any, opts ConvertOptions) error {
	raw, err := yaml.Marshal(model)
	if err != nil {
		return err
	}

	dir := os.TempDir()
	composeYaml := filepath.Join(dir, "compose.yaml")
	err = os.WriteFile(composeYaml, raw, 0o600)
	if err != nil {
		return err
	}

	out, err := filepath.Abs(opts.Output)
	if err != nil {
		return err
	}
	binds := []string{
		fmt.Sprintf("%s:%s", dir, "/in"),
		fmt.Sprintf("%s:%s", out, "/out"),
	}
	if opts.Templates != "" {
		templateDir, err := filepath.Abs(opts.Templates)
		if err != nil {
			return err
		}
		binds = append(binds, fmt.Sprintf("%s:%s", templateDir, "/templates"))
	}

	for _, transformation := range opts.Transformations {
		_, err = inspectWithPull(ctx, dockerCli, transformation)
		if err != nil {
			return err
		}

		usr, err := user.Current()
		if err != nil {
			return err
		}
		created, err := dockerCli.Client().ContainerCreate(ctx, &container.Config{
			Image: transformation,
			Env:   []string{"LICENSE_AGREEMENT=true"},
			User:  usr.Uid,
		}, &container.HostConfig{
			AutoRemove: true,
			Binds:      binds,
		}, &network.NetworkingConfig{}, nil, "")
		if err != nil {
			return err
		}

		err = cli.RunStart(ctx, dockerCli, &cli.StartOptions{
			Attach:     true,
			Containers: []string{created.ID},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// LoadAdditionalResources loads additional resources from the project, such as image references, secrets, configs and exposed ports
func LoadAdditionalResources(ctx context.Context, dockerCLI command.Cli, project *types.Project) (*types.Project, error) {
	for name, service := range project.Services {
		imageName := api.GetImageNameOrDefault(service, project.Name)

		inspect, err := inspectWithPull(ctx, dockerCLI, imageName)
		if err != nil {
			return nil, err
		}
		service.Image = imageName
		exposed := utils.Set[string]{}
		exposed.AddAll(service.Expose...)
		for port := range inspect.Config.ExposedPorts {
			exposed.Add(nat.Port(port).Port())
		}
		for _, port := range service.Ports {
			exposed.Add(strconv.Itoa(int(port.Target)))
		}
		service.Expose = exposed.Elements()
		project.Services[name] = service
	}

	for name, secret := range project.Secrets {
		f, err := loadFileObject(types.FileObjectConfig(secret))
		if err != nil {
			return nil, err
		}
		project.Secrets[name] = types.SecretConfig(f)
	}

	for name, config := range project.Configs {
		f, err := loadFileObject(types.FileObjectConfig(config))
		if err != nil {
			return nil, err
		}
		project.Configs[name] = types.ConfigObjConfig(f)
	}

	return project, nil
}

func loadFileObject(conf types.FileObjectConfig) (types.FileObjectConfig, error) {
	if !conf.External {
		switch {
		case conf.Environment != "":
			conf.Content = os.Getenv(conf.Environment)
		case conf.File != "":
			bytes, err := os.ReadFile(conf.File)
			if err != nil {
				return conf, err
			}
			conf.Content = string(bytes)
		}
	}
	return conf, nil
}

func inspectWithPull(ctx context.Context, dockerCli command.Cli, imageName string) (image.InspectResponse, error) {
	inspect, err := dockerCli.Client().ImageInspect(ctx, imageName)
	if cerrdefs.IsNotFound(err) {
		var stream io.ReadCloser
		stream, err = dockerCli.Client().ImagePull(ctx, imageName, image.PullOptions{})
		if err != nil {
			return image.InspectResponse{}, err
		}
		defer func() { _ = stream.Close() }()

		err = jsonmessage.DisplayJSONMessagesToStream(stream, dockerCli.Out(), nil)
		if err != nil {
			return image.InspectResponse{}, err
		}
		if inspect, err = dockerCli.Client().ImageInspect(ctx, imageName); err != nil {
			return image.InspectResponse{}, err
		}
	}
	return inspect, err
}
