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

package compose

import (
	"context"
	"io"

	"github.com/compose-spec/compose-go/types"
)

// Service manages a compose project
type Service interface {
	// Build executes the equivalent to a `compose build`
	Build(ctx context.Context, project *types.Project) error
	// Push executes the equivalent ot a `compose push`
	Push(ctx context.Context, project *types.Project) error
	// Pull executes the equivalent of a `compose pull`
	Pull(ctx context.Context, project *types.Project) error
	// Create executes the equivalent to a `compose create`
	Create(ctx context.Context, project *types.Project, opts CreateOptions) error
	// Start executes the equivalent to a `compose start`
	Start(ctx context.Context, project *types.Project, consumer LogConsumer) error
	// Up executes the equivalent to a `compose up`
	Up(ctx context.Context, project *types.Project, options UpOptions) error
	// Down executes the equivalent to a `compose down`
	Down(ctx context.Context, projectName string, options DownOptions) error
	// Logs executes the equivalent to a `compose logs`
	Logs(ctx context.Context, projectName string, consumer LogConsumer, options LogOptions) error
	// Ps executes the equivalent to a `compose ps`
	Ps(ctx context.Context, projectName string) ([]ContainerSummary, error)
	// List executes the equivalent to a `docker stack ls`
	List(ctx context.Context) ([]Stack, error)
	// Convert translate compose model into backend's native format
	Convert(ctx context.Context, project *types.Project, options ConvertOptions) ([]byte, error)
	// RunOneOffContainer creates a service oneoff container and starts its dependencies
	RunOneOffContainer(ctx context.Context, project *types.Project, opts RunOptions) error
}

// CreateOptions group options of the Create API
type CreateOptions struct {
	// Remove legacy containers for services that are not defined in the project
	RemoveOrphans bool
}

// UpOptions group options of the Up API
type UpOptions struct {
	// Detach will create services and return immediately
	Detach bool
}

// DownOptions group options of the Down API
type DownOptions struct {
	// RemoveOrphans will cleanup containers that are not declared on the compose model but own the same labels
	RemoveOrphans bool
	// Project is the compose project used to define this app. Might be nil if user ran `down` just with project name
	Project *types.Project
}

// ConvertOptions group options of the Convert API
type ConvertOptions struct {
	// Format define the output format used to dump converted application model (json|yaml)
	Format string
}

// RunOptions options to execute compose run
type RunOptions struct {
	Service    string
	Command    []string
	Detach     bool
	AutoRemove bool
	Writer     io.Writer
	Reader     io.Reader
}

// PortPublisher hold status about published port
type PortPublisher struct {
	URL           string
	TargetPort    int
	PublishedPort int
	Protocol      string
}

// ContainerSummary hold high-level description of a container
type ContainerSummary struct {
	ID         string
	Name       string
	Project    string
	Service    string
	State      string
	Publishers []PortPublisher
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

// LogOptions defines optional parameters for the `Log` API
type LogOptions struct {
	Services []string
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
	Reason string
}

// LogConsumer is a callback to process log messages from services
type LogConsumer interface {
	Log(service, container, message string)
}
