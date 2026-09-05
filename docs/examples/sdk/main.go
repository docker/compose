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

// This file is the first fenced Go block of docs/sdk.md, verbatim from the
// package clause down: sdk_md_test.go fails if the two ever drift, and the
// compiler keeps the documented API real.

package main

import (
	"context"
	"log"

	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/flags"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/compose"
)

func main() {
	ctx := context.Background()

	dockerCLI, err := command.NewDockerCli()
	if err != nil {
		log.Fatalf("Failed to create docker CLI: %v", err)
	}
	err = dockerCLI.Initialize(&flags.ClientOptions{})
	if err != nil {
		log.Fatalf("Failed to initialize docker CLI: %v", err)
	}

	// Create a new Compose service instance
	service, err := compose.NewComposeService(dockerCLI)
	if err != nil {
		log.Fatalf("Failed to create compose service: %v", err)
	}

	// Load the Compose project from a compose file
	project, err := service.LoadProject(ctx, api.ProjectLoadOptions{
		ConfigPaths: []string{"compose.yaml"},
		ProjectName: "my-app",
	})
	if err != nil {
		log.Fatalf("Failed to load project: %v", err)
	}

	// Start the services defined in the Compose file
	err = service.Up(ctx, project, api.UpOptions{
		Create: api.CreateOptions{},
		Start:  api.StartOptions{},
	})
	if err != nil {
		log.Fatalf("Failed to start services: %v", err)
	}

	log.Printf("Successfully started project: %s", project.Name)
}
