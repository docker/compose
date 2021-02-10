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
	Start(ctx context.Context, project *types.Project, options StartOptions) error
	// Stop executes the equivalent to a `compose stop`
	Stop(ctx context.Context, project *types.Project) error
	// Up executes the equivalent to a `compose up`
	Up(ctx context.Context, project *types.Project, options UpOptions) error
	// Down executes the equivalent to a `compose down`
	Down(ctx context.Context, projectName string, options DownOptions) error
	// Logs executes the equivalent to a `compose logs`
	Logs(ctx context.Context, projectName string, consumer LogConsumer, options LogOptions) error
	// Ps executes the equivalent to a `compose ps`
	Ps(ctx context.Context, projectName string, options PsOptions) ([]ContainerSummary, error)
	// List executes the equivalent to a `docker stack ls`
	List(ctx context.Context) ([]Stack, error)
	// Convert translate compose model into backend's native format
	Convert(ctx context.Context, project *types.Project, options ConvertOptions) ([]byte, error)
	// Kill executes the equivalent to a `compose kill`
	Kill(ctx context.Context, project *types.Project, options KillOptions) error
	// RunOneOffContainer creates a service oneoff container and starts its dependencies
	RunOneOffContainer(ctx context.Context, project *types.Project, opts RunOptions) error
}

// CreateOptions group options of the Create API
type CreateOptions struct {
	// Remove legacy containers for services that are not defined in the project
	RemoveOrphans bool
	// Recreate define the strategy to apply on existing containers
	Recreate string
}

// StartOptions group options of the Start API
type StartOptions struct {
	// Attach will attach to service containers and pipe stdout/stderr to channel
	Attach ContainerEventListener
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
	// Output defines the path to save the application model
	Output string
}

// KillOptions group options of the Kill API
type KillOptions struct {
	// Signal to send to containers
	Signal string
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

// PsOptions group options of the Ps API
type PsOptions struct {
	All bool
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
	Health     string
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
	Tail     string
	Follow   bool
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

const (
	// RecreateDiverged to recreate services which configuration diverges from compose model
	RecreateDiverged = "diverged"
	// RecreateForce to force service container being recreated
	RecreateForce = "force"
	// RecreateNever to never recreate existing service containers
	RecreateNever = "never"
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
	Status(service, container, msg string)
}

// ContainerEventListener is a callback to process ContainerEvent from services
type ContainerEventListener func(event ContainerEvent)

// ContainerEvent notify an event has been collected on Source container implementing Service
type ContainerEvent struct {
	Type     int
	Source   string
	Service  string
	Line     string
	ExitCode int
}

const (
	// ContainerEventLog is a ContainerEvent of type log. Line is set
	ContainerEventLog = iota
	// ContainerEventExit is a ContainerEvent of type exit. ExitCode is set
	ContainerEventExit
)
