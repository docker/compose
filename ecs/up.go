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
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/compose-spec/compose-go/types"
	"github.com/docker/compose-cli/errdefs"
)

func (b *ecsAPIService) Build(ctx context.Context, project *types.Project) error {
	return errdefs.ErrNotImplemented
}

func (b *ecsAPIService) Push(ctx context.Context, project *types.Project) error {
	return errdefs.ErrNotImplemented
}

func (b *ecsAPIService) Pull(ctx context.Context, project *types.Project) error {
	return errdefs.ErrNotImplemented
}

func (b *ecsAPIService) Create(ctx context.Context, project *types.Project) error {
	return errdefs.ErrNotImplemented
}

func (b *ecsAPIService) Start(ctx context.Context, project *types.Project, w io.Writer) error {
	return errdefs.ErrNotImplemented
}

func (b *ecsAPIService) Up(ctx context.Context, project *types.Project, detach bool) error {

	err := b.aws.CheckRequirements(ctx, b.Region)
	if err != nil {
		return err
	}

	template, err := b.Convert(ctx, project, "yaml")
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
	if detach {
		return nil
	}
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-signalChan
		fmt.Println("user interrupted deployment. Deleting stack...")
		b.Down(ctx, project.Name) // nolint:errcheck
	}()

	err = b.WaitStackCompletion(ctx, project.Name, operation)
	return err
}
