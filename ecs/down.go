/*
   Copyright 2020 Docker, Inc.

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

	"github.com/compose-spec/compose-go/cli"
)

func (b *ecsAPIService) Down(ctx context.Context, options *cli.ProjectOptions) error {
	name, err := b.projectName(options)
	if err != nil {
		return err
	}

	err = b.SDK.DeleteStack(ctx, name)
	if err != nil {
		return err
	}
	return b.WaitStackCompletion(ctx, name, StackDelete)
}

func (b *ecsAPIService) projectName(options *cli.ProjectOptions) (string, error) {
	name := options.Name
	if name == "" {
		project, err := cli.ProjectFromOptions(options)
		if err != nil {
			return "", err
		}
		name = project.Name
	}
	return name, nil
}
