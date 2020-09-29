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

	"github.com/compose-spec/compose-go/types"
)

func (b *ecsAPIService) Up(ctx context.Context, project *types.Project) error {
	err := b.SDK.CheckRequirements(ctx, b.Region)
	if err != nil {
		return err
	}

	template, err := b.Convert(ctx, project)
	if err != nil {
		return err
	}

	update, err := b.SDK.StackExists(ctx, project.Name)
	if err != nil {
		return err
	}
	operation := stackCreate
	if update {
		operation = stackUpdate
		changeset, err := b.SDK.CreateChangeSet(ctx, project.Name, template)
		if err != nil {
			return err
		}
		err = b.SDK.UpdateStack(ctx, changeset)
		if err != nil {
			return err
		}
	} else {
		err = b.SDK.CreateStack(ctx, project.Name, template)
		if err != nil {
			return err
		}
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
