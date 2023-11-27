/*
   Copyright 2023 Docker Compose CLI authors

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

package sync

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/sirupsen/logrus"
)

type ComposeClient interface {
	Exec(ctx context.Context, projectName string, options api.RunOptions) (int, error)

	Copy(ctx context.Context, projectName string, options api.CopyOptions) error
}

type DockerCopy struct {
	client ComposeClient

	projectName string

	infoWriter io.Writer
}

var _ Syncer = &DockerCopy{}

func NewDockerCopy(projectName string, client ComposeClient, infoWriter io.Writer) *DockerCopy {
	return &DockerCopy{
		projectName: projectName,
		client:      client,
		infoWriter:  infoWriter,
	}
}

func (d *DockerCopy) Sync(ctx context.Context, service types.ServiceConfig, paths []PathMapping) error {
	var errs []error
	for i := range paths {
		if err := d.sync(ctx, service, paths[i]); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (d *DockerCopy) sync(ctx context.Context, service types.ServiceConfig, pathMapping PathMapping) error {
	scale := service.GetScale()

	if fi, statErr := os.Stat(pathMapping.HostPath); statErr == nil {
		if fi.IsDir() {
			for i := 1; i <= scale; i++ {
				_, err := d.client.Exec(ctx, d.projectName, api.RunOptions{
					Service: service.Name,
					Command: []string{"mkdir", "-p", pathMapping.ContainerPath},
					Index:   i,
				})
				if err != nil {
					logrus.Warnf("failed to create %q from %s: %v", pathMapping.ContainerPath, service.Name, err)
				}
			}
			fmt.Fprintf(d.infoWriter, "%s created\n", pathMapping.ContainerPath)
		} else {
			err := d.client.Copy(ctx, d.projectName, api.CopyOptions{
				Source:      pathMapping.HostPath,
				Destination: fmt.Sprintf("%s:%s", service.Name, pathMapping.ContainerPath),
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(d.infoWriter, "%s updated\n", pathMapping.ContainerPath)
		}
	} else if errors.Is(statErr, fs.ErrNotExist) {
		for i := 1; i <= scale; i++ {
			_, err := d.client.Exec(ctx, d.projectName, api.RunOptions{
				Service: service.Name,
				Command: []string{"rm", "-rf", pathMapping.ContainerPath},
				Index:   i,
			})
			if err != nil {
				logrus.Warnf("failed to delete %q from %s: %v", pathMapping.ContainerPath, service.Name, err)
			}
		}
		fmt.Fprintf(d.infoWriter, "%s deleted from service\n", pathMapping.ContainerPath)
	}
	return nil
}
