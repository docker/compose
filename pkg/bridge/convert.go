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
	"runtime"
	"strconv"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/containerd/errdefs"
	"github.com/docker/cli/cli/command"
	cli "github.com/docker/cli/cli/command/container"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"github.com/moby/moby/client/pkg/jsonmessage"
	"github.com/sirupsen/logrus"
	"go.yaml.in/yaml/v4"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/utils"
)

// Confirm asks the user to confirm a destructive action described by
// message, returning defaultValue when no explicit answer can be read.
type Confirm func(message string, defaultValue bool) (bool, error)

type ConvertOptions struct {
	Output          string
	Templates       string
	Transformations []string
	// Confirm is asked before deleting a non-empty output directory. If nil,
	// deletion is refused whenever the directory is not empty.
	Confirm Confirm
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
		if err := prepareOutputDir(opts); err != nil {
			return err
		}
	}
	// Run Transformers images
	return convert(ctx, dockerCli, model, opts)
}

// prepareOutputDir makes sure output exists and is empty. If it already
// contains files, it asks for confirmation before deleting them, so a typo
// or misuse (e.g. -o . or -o $HOME) doesn't silently destroy user data.
func prepareOutputDir(opts ConvertOptions) error {
	empty, err := isEmptyOrMissingDir(opts.Output)
	if err != nil {
		return err
	}
	if empty {
		return os.MkdirAll(opts.Output, 0o744)
	}

	confirmed := false
	if opts.Confirm != nil {
		confirmed, err = opts.Confirm(
			fmt.Sprintf("Output directory '%s' is not empty, all its content will be permanently deleted. Continue?", opts.Output),
			false)
		if err != nil {
			return err
		}
	}
	if !confirmed {
		return fmt.Errorf("deletion of output directory '%s' was not confirmed", opts.Output)
	}
	if err := os.RemoveAll(opts.Output); err != nil {
		return fmt.Errorf("cannot remove existing output folder: %w", err)
	}
	if err := os.MkdirAll(opts.Output, 0o744); err != nil {
		return fmt.Errorf("output directory '%s' was deleted but could not be recreated: %w", opts.Output, err)
	}
	return nil
}

func isEmptyOrMissingDir(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, fmt.Errorf("cannot read output folder: %w", err)
	}
	return len(entries) == 0, nil
}

func convert(ctx context.Context, dockerCli command.Cli, model map[string]any, opts ConvertOptions) error {
	raw, err := yaml.Marshal(model)
	if err != nil {
		return err
	}

	dir, err := os.MkdirTemp("", "compose-convert-*")
	if err != nil {
		return err
	}
	defer func() {
		err := os.RemoveAll(dir)
		if err != nil {
			logrus.Warnf("failed to remove temp dir %s: %v", dir, err)
		}
	}()

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

		containerConfig := &container.Config{
			Image: transformation,
			Env:   []string{"LICENSE_AGREEMENT=true"},
		}
		// On POSIX systems, this is a decimal number representing the uid.
		// On Windows, this is a security identifier (SID) in a string format and the engine isn't able to manage it
		if runtime.GOOS != "windows" {
			usr, err := user.Current()
			if err != nil {
				return err
			}
			containerConfig.User = usr.Uid
		}
		created, err := dockerCli.Client().ContainerCreate(ctx, client.ContainerCreateOptions{
			Config: containerConfig,
			HostConfig: &container.HostConfig{
				Binds:      binds,
				AutoRemove: true,
			},
			NetworkingConfig: &network.NetworkingConfig{},
		})
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
		updated, err := loadServiceImageResources(ctx, dockerCLI, project.Name, name, service)
		if err != nil {
			return nil, err
		}
		project.Services[name] = updated
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

// loadServiceImageResources resolves the service image and merges the ports
// it exposes into the service's Expose list
func loadServiceImageResources(ctx context.Context, dockerCLI command.Cli, projectName, name string, service types.ServiceConfig) (types.ServiceConfig, error) {
	imageName := api.GetImageNameOrDefault(service, projectName)

	inspect, err := inspectServiceImage(ctx, dockerCLI, name, imageName, service)
	if err != nil {
		return service, err
	}

	service.Image = imageName
	exposed := utils.Set[string]{}
	exposed.AddAll(service.Expose...)
	if inspect.Config != nil {
		for port := range inspect.Config.ExposedPorts {
			p, err := network.ParsePort(port)
			if err != nil {
				return service, err
			}
			exposed.Add(strconv.Itoa(int(p.Num())))
		}
	}
	for _, port := range service.Ports {
		exposed.Add(strconv.Itoa(int(port.Target)))
	}
	service.Expose = exposed.Elements()
	return service, nil
}

// inspectServiceImage inspects the service image, pulling it when needed; a
// buildable image missing locally only degrades to a warning
func inspectServiceImage(ctx context.Context, dockerCLI command.Cli, name, imageName string, service types.ServiceConfig) (image.InspectResponse, error) {
	if service.Build != nil && service.Image == "" {
		result, err := dockerCLI.Client().ImageInspect(ctx, imageName)
		if err != nil {
			if !errdefs.IsNotFound(err) {
				return image.InspectResponse{}, err
			}
			logrus.Warnf("image %s for service %s not found locally; Dockerfile-exposed ports will not be included — run `docker compose build` first to include them", imageName, name)
		}
		return result.InspectResponse, nil
	}
	return inspectWithPull(ctx, dockerCLI, imageName)
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
	if errdefs.IsNotFound(err) {
		var stream io.ReadCloser
		stream, err = dockerCli.Client().ImagePull(ctx, imageName, client.ImagePullOptions{})
		if err != nil {
			return image.InspectResponse{}, err
		}
		defer func() { _ = stream.Close() }()

		out := dockerCli.Out()
		err = jsonmessage.DisplayJSONMessagesStream(stream, out, out.FD(), out.IsTerminal(), nil)
		if err != nil {
			return image.InspectResponse{}, err
		}
		if inspect, err = dockerCli.Client().ImageInspect(ctx, imageName); err != nil {
			return image.InspectResponse{}, err
		}
	}
	return inspect.InspectResponse, err
}
