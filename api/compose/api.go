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

package compose

import (
	"context"
	"io"

	"github.com/compose-spec/compose-go/types"
)

// Service manages a compose project
type Service interface {
	// Up executes the equivalent to a `compose up`
	Up(ctx context.Context, project *types.Project) error
	// Down executes the equivalent to a `compose down`
	Down(ctx context.Context, projectName string) error
	// Logs executes the equivalent to a `compose logs`
	Logs(ctx context.Context, projectName string, w io.Writer) error
	// Ps executes the equivalent to a `compose ps`
	Ps(ctx context.Context, projectName string) ([]ServiceStatus, error)
	// List executes the equivalent to a `docker stack ls`
	List(ctx context.Context, projectName string) ([]Stack, error)
	// Convert translate compose model into backend's native format
	Convert(ctx context.Context, project *types.Project) ([]byte, error)
}

// PortPublisher hold status about published port
type PortPublisher struct {
	URL           string
	TargetPort    int
	PublishedPort int
	Protocol      string
}

// ServiceStatus hold status about a service
type ServiceStatus struct {
	ID         string
	Name       string
	Replicas   int
	Desired    int
	Ports      []string
	Publishers []PortPublisher
}

const (
	// STARTING indicates that stack is being deployed
	STARTING string = "Starting"
	// RUNNING indicates that stack is deployed and services are running
	RUNNING string = "Running"
	// UPDATING indicates that some stack resources are being recreated
	UPDATING string = "Updating"
	// REMOVING indicates that stack is being deleted
	REMOVING string = "Removing"
	// UNKNOWN indicates unknown stack state
	UNKNOWN string = "Unknown"
	// FAILED indicates that stack deployment failed
	FAILED string = "Failed"
)

// Stack holds the name and state of a compose application/stack
type Stack struct {
	ID     string
	Name   string
	Status string
}
