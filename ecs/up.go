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

package ecs

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"

	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/api/errdefs"

	"github.com/compose-spec/compose-go/types"
)

func (b *ecsAPIService) Build(ctx context.Context, project *types.Project, options compose.BuildOptions) error {
	return errdefs.ErrNotImplemented
}

func (b *ecsAPIService) Push(ctx context.Context, project *types.Project, options compose.PushOptions) error {
	return errdefs.ErrNotImplemented
}

func (b *ecsAPIService) Pull(ctx context.Context, project *types.Project) error {
	return errdefs.ErrNotImplemented
}

func (b *ecsAPIService) Create(ctx context.Context, project *types.Project, opts compose.CreateOptions) error {
	return errdefs.ErrNotImplemented
}

func (b *ecsAPIService) Start(ctx context.Context, project *types.Project, options compose.StartOptions) error {
	return errdefs.ErrNotImplemented
}

func (b *ecsAPIService) Stop(ctx context.Context, project *types.Project, options compose.StopOptions) error {
	return errdefs.ErrNotImplemented
}

func (b *ecsAPIService) Pause(ctx context.Context, project *types.Project) error {
	return errdefs.ErrNotImplemented
}

func (b *ecsAPIService) UnPause(ctx context.Context, project *types.Project) error {
	return errdefs.ErrNotImplemented
}

func (b *ecsAPIService) Events(ctx context.Context, project string, options compose.EventsOptions) error {
	return errdefs.ErrNotImplemented
}

func (b *ecsAPIService) Port(ctx context.Context, project string, service string, port int, options compose.PortOptions) (string, int, error) {
	return "", 0, errdefs.ErrNotImplemented
}

func (b *ecsAPIService) Up(ctx context.Context, project *types.Project, options compose.UpOptions) error {
	logrus.Debugf("deploying on AWS with region=%q", b.Region)
	err := b.aws.CheckRequirements(ctx, b.Region)
	if err != nil {
		return err
	}

	template, err := b.Convert(ctx, project, compose.ConvertOptions{
		Format: "yaml",
	})
	if err != nil {
		return err
	}

	update, err := b.aws.StackExists(ctx, project.Name)
	if err != nil {
		return err
	}
	operation := stackCreate
	if update {
		operation = stackUpdate
		changeset, err := b.aws.CreateChangeSet(ctx, project.Name, b.Region, template)
		if err != nil {
			return err
		}
		err = b.aws.UpdateStack(ctx, changeset)
		if err != nil {
			return err
		}
	} else {
		err = b.aws.CreateStack(ctx, project.Name, b.Region, template)
		if err != nil {
			return err
		}
	}
	if options.Detach {
		return nil
	}
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-signalChan
		fmt.Println("user interrupted deployment. Deleting stack...")
		b.Down(ctx, project.Name, compose.DownOptions{}) // nolint:errcheck
	}()

	err = b.WaitStackCompletion(ctx, project.Name, operation)
	return err
}
