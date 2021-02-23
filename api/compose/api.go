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
	"strings"
	"time"

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
	Stop(ctx context.Context, project *types.Project, options StopOptions) error
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
	RunOneOffContainer(ctx context.Context, project *types.Project, opts RunOptions) (int, error)
	// Remove executes the equivalent to a `compose rm`
	Remove(ctx context.Context, project *types.Project, options RemoveOptions) ([]string, error)
	// Exec executes a command in a running service container
	Exec(ctx context.Context, project *types.Project, opts RunOptions) error
	// Pause executes the equivalent to a `compose pause`
	Pause(ctx context.Context, project *types.Project) error
	// UnPause executes the equivalent to a `compose unpause`
	UnPause(ctx context.Context, project *types.Project) error
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
	// Attach will attach to service containers and send container logs and events
	Attach ContainerEventListener
}

// StopOptions group options of the Stop API
type StopOptions struct {
	// Timeout override container stop timeout
	Timeout *time.Duration
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
	// Timeout override container stop timeout
	Timeout *time.Duration
	// Images remove image used by services. 'all': Remove all images. 'local': Remove only images that don't have a tag
	Images string
	// Volumes remove volumes, both declared in the `volumes` section and anonymous ones
	Volumes bool
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

// RemoveOptions group options of the Remove API
type RemoveOptions struct {
	// DryRun just list removable resources
	DryRun bool
	// Volumes remove anonymous volumes
	Volumes bool
	// Force don't ask to confirm removal
	Force bool
}

// RunOptions options to execute compose run
type RunOptions struct {
	Name        string
	Service     string
	Command     []string
	Entrypoint  []string
	Detach      bool
	AutoRemove  bool
	Writer      io.Writer
	Reader      io.Reader
	Tty         bool
	WorkingDir  string
	User        string
	Environment []string
	Labels      types.Labels
	Privileged  bool
	// used by exec
	Index int
}

// EnvironmentMap return RunOptions.Environment as a MappingWithEquals
func (opts *RunOptions) EnvironmentMap() types.MappingWithEquals {
	environment := types.MappingWithEquals{}
	for _, s := range opts.Environment {
		parts := strings.SplitN(s, "=", 2)
		key := parts[0]
		switch {
		case len(parts) == 1:
			environment[key] = nil
		default:
			environment[key] = &parts[1]
		}
	}
	return environment
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
	Services   []string
	Tail       string
	Follow     bool
	Timestamps bool
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
	Log(name, service, container, message string)
	Status(name, container, msg string)
	Register(name string, source string)
}

// ContainerEventListener is a callback to process ContainerEvent from services
type ContainerEventListener func(event ContainerEvent)

// ContainerEvent notify an event has been collected on Source container implementing Service
type ContainerEvent struct {
	Type     int
	Source   string
	Service  string
	Name     string
	Line     string
	ExitCode int
}

const (
	// ContainerEventLog is a ContainerEvent of type log. Line is set
	ContainerEventLog = iota
	// ContainerEventAttach is a ContainerEvent of type attach. First event sent about a container
	ContainerEventAttach
	// ContainerEventExit is a ContainerEvent of type exit. ExitCode is set
	ContainerEventExit
	// UserCancel user cancelled compose up, we are stopping containers
	UserCancel
)
