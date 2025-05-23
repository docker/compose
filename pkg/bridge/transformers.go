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
	"os"
	"path/filepath"

	"github.com/docker/cli/cli/command"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/moby/go-archive"
)

const (
	TransformerLabel        = "com.docker.compose.bridge"
	DefaultTransformerImage = "docker/compose-bridge-kubernetes"
)

type CreateTransformerOptions struct {
	Dest string
	From string
}

func CreateTransformer(ctx context.Context, dockerCli command.Cli, options CreateTransformerOptions) error {
	if options.From == "" {
		options.From = DefaultTransformerImage
	}
	out, err := filepath.Abs(options.Dest)
	if err != nil {
		return err
	}

	if _, err := os.Stat(out); err == nil {
		return fmt.Errorf("output folder %s already exists", out)
	}

	tmpl := filepath.Join(out, "templates")
	err = os.MkdirAll(tmpl, 0o744)
	if err != nil && !os.IsExist(err) {
		return fmt.Errorf("cannot create output folder: %w", err)
	}

	if err := command.ValidateOutputPath(out); err != nil {
		return err
	}

	created, err := dockerCli.Client().ContainerCreate(ctx, &container.Config{
		Image: options.From,
	}, &container.HostConfig{}, &network.NetworkingConfig{}, nil, "")
	defer func() {
		_ = dockerCli.Client().ContainerRemove(context.Background(), created.ID, container.RemoveOptions{Force: true})
	}()

	if err != nil {
		return err
	}
	content, stat, err := dockerCli.Client().CopyFromContainer(ctx, created.ID, "/templates")
	if err != nil {
		return err
	}
	defer func() {
		_ = content.Close()
	}()

	srcInfo := archive.CopyInfo{
		Path:   "/templates",
		Exists: true,
		IsDir:  stat.Mode.IsDir(),
	}

	preArchive := content
	if srcInfo.RebaseName != "" {
		_, srcBase := archive.SplitPathDirEntry(srcInfo.Path)
		preArchive = archive.RebaseArchiveEntries(content, srcBase, srcInfo.RebaseName)
	}

	if err := archive.CopyTo(preArchive, srcInfo, out); err != nil {
		return err
	}

	dockerfile := `FROM docker/compose-bridge-transformer
LABEL com.docker.compose.bridge=transformation
COPY templates /templates
`
	if err := os.WriteFile(filepath.Join(out, "Dockerfile"), []byte(dockerfile), 0o700); err != nil {
		return err
	}
	_, err = fmt.Fprintf(dockerCli.Out(), "Transformer created in %q\n", out)
	return err
}

func ListTransformers(ctx context.Context, dockerCli command.Cli) ([]image.Summary, error) {
	api := dockerCli.Client()
	return api.ImageList(ctx, image.ListOptions{
		Filters: filters.NewArgs(
			filters.Arg("label", fmt.Sprintf("%s=%s", TransformerLabel, "transformation")),
		),
	})
}
