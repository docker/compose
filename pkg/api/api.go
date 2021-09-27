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

package api

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/compose-spec/compose-go/types"
)

// Service manages a compose project
type Service interface {
	// Build executes the equivalent to a `compose build`
	Build(ctx context.Context, project *types.Project, options BuildOptions) error
	// Push executes the equivalent ot a `compose push`
	Push(ctx context.Context, project *types.Project, options PushOptions) error
	// Pull executes the equivalent of a `compose pull`
	Pull(ctx context.Context, project *types.Project, opts PullOptions) error
	// Create executes the equivalent to a `compose create`
	Create(ctx context.Context, project *types.Project, opts CreateOptions) error
	// Start executes the equivalent to a `compose start`
	Start(ctx context.Context, project *types.Project, options StartOptions) error
	// Restart restarts containers
	Restart(ctx context.Context, project *types.Project, options RestartOptions) error
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
	List(ctx context.Context, options ListOptions) ([]Stack, error)
	// Convert translate compose model into backend's native format
	Convert(ctx context.Context, project *types.Project, options ConvertOptions) ([]byte, error)
	// Kill executes the equivalent to a `compose kill`
	Kill(ctx context.Context, project *types.Project, options KillOptions) error
	// RunOneOffContainer creates a service oneoff container and starts its dependencies
	RunOneOffContainer(ctx context.Context, project *types.Project, opts RunOptions) (int, error)
	// Remove executes the equivalent to a `compose rm`
	Remove(ctx context.Context, project *types.Project, options RemoveOptions) error
	// Exec executes a command in a running service container
	Exec(ctx context.Context, project string, opts RunOptions) (int, error)
	// Copy copies a file/folder between a service container and the local filesystem
	Copy(ctx context.Context, project *types.Project, opts CopyOptions) error
	// Pause executes the equivalent to a `compose pause`
	Pause(ctx context.Context, project string, options PauseOptions) error
	// UnPause executes the equivalent to a `compose unpause`
	UnPause(ctx context.Context, project string, options PauseOptions) error
	// Top executes the equivalent to a `compose top`
	Top(ctx context.Context, projectName string, services []string) ([]ContainerProcSummary, error)
	// Events executes the equivalent to a `compose events`
	Events(ctx context.Context, project string, options EventsOptions) error
	// Port executes the equivalent to a `compose port`
	Port(ctx context.Context, project string, service string, port int, options PortOptions) (string, int, error)
	// Images executes the equivalent of a `compose images`
	Images(ctx context.Context, projectName string, options ImagesOptions) ([]ImageSummary, error)
}

// BuildOptions group options of the Build API
type BuildOptions struct {
	// Pull always attempt to pull a newer version of the image
	Pull bool
	// Progress set type of progress output ("auto", "plain", "tty")
	Progress string
	// Args set build-time args
	Args types.MappingWithEquals
	// NoCache disables cache use
	NoCache bool
	// Quiet make the build process not output to the console
	Quiet bool
	// Services passed in the command line to be built
	Services []string
}

// CreateOptions group options of the Create API
type CreateOptions struct {
	// Services defines the services user interacts with
	Services []string
	// Remove legacy containers for services that are not defined in the project
	RemoveOrphans bool
	// Ignore legacy containers for services that are not defined in the project
	IgnoreOrphans bool
	// Recreate define the strategy to apply on existing containers
	Recreate string
	// RecreateDependencies define the strategy to apply on dependencies services
	RecreateDependencies string
	// Inherit reuse anonymous volumes from previous container
	Inherit bool
	// Timeout set delay to wait for container to gracelfuly stop before sending SIGKILL
	Timeout *time.Duration
	// QuietPull makes the pulling process quiet
	QuietPull bool
}

// StartOptions group options of the Start API
type StartOptions struct {
	// Attach to container and forward logs if not nil
	Attach LogConsumer
	// AttachTo set the services to attach to
	AttachTo []string
	// CascadeStop stops the application when a container stops
	CascadeStop bool
	// ExitCodeFrom return exit code from specified service
	ExitCodeFrom string
}

// RestartOptions group options of the Restart API
type RestartOptions struct {
	// Timeout override container restart timeout
	Timeout *time.Duration
	// Services passed in the command line to be restarted
	Services []string
}

// StopOptions group options of the Stop API
type StopOptions struct {
	// Timeout override container stop timeout
	Timeout *time.Duration
	// Services passed in the command line to be stopped
	Services []string
}

// UpOptions group options of the Up API
type UpOptions struct {
	Create CreateOptions
	Start  StartOptions
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

// PushOptions group options of the Push API
type PushOptions struct {
	IgnoreFailures bool
}

// PullOptions group options of the Pull API
type PullOptions struct {
	Quiet          bool
	IgnoreFailures bool
}

// ImagesOptions group options of the Images API
type ImagesOptions struct {
	Services []string
}

// KillOptions group options of the Kill API
type KillOptions struct {
	// Services passed in the command line to be killed
	Services []string
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
	// Services passed in the command line to be removed
	Services []string
}

// RunOptions group options of the Run API
type RunOptions struct {
	Name              string
	Service           string
	Command           []string
	Entrypoint        []string
	Detach            bool
	AutoRemove        bool
	Stdin             io.ReadCloser
	Stdout            io.WriteCloser
	Stderr            io.WriteCloser
	Tty               bool
	WorkingDir        string
	User              string
	Environment       []string
	Labels            types.Labels
	Privileged        bool
	UseNetworkAliases bool
	NoDeps            bool
	// used by exec
	Index int
}

// EventsOptions group options of the Events API
type EventsOptions struct {
	Services []string
	Consumer func(event Event) error
}

// Event is a container runtime event served by Events API
type Event struct {
	Timestamp  time.Time
	Service    string
	Container  string
	Status     string
	Attributes map[string]string
}

// PortOptions group options of the Port API
type PortOptions struct {
	Protocol string
	Index    int
}

func (e Event) String() string {
	t := e.Timestamp.Format("2006-01-02 15:04:05.000000")
	var attr []string
	for k, v := range e.Attributes {
		attr = append(attr, fmt.Sprintf("%s=%s", k, v))
	}
	return fmt.Sprintf("%s container %s %s (%s)\n", t, e.Status, e.Container, strings.Join(attr, ", "))

}

// ListOptions group options of the ls API
type ListOptions struct {
	All bool
}

// PsOptions group options of the Ps API
type PsOptions struct {
	All      bool
	Services []string
}

// CopyOptions group options of the cp API
type CopyOptions struct {
	Source      string
	Destination string
	All         bool
	Index       int
	FollowLink  bool
	CopyUIDGID  bool
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
	Command    string
	Project    string
	Service    string
	State      string
	Health     string
	ExitCode   int
	Publishers PortPublishers
}

// PortPublishers is a slice of PortPublisher
type PortPublishers []PortPublisher

// Len implements sort.Interface
func (p PortPublishers) Len() int {
	return len(p)
}

// Less implements sort.Interface
func (p PortPublishers) Less(i, j int) bool {
	left := p[i]
	right := p[j]
	if left.URL != right.URL {
		return left.URL < right.URL
	}
	if left.TargetPort != right.TargetPort {
		return left.TargetPort < right.TargetPort
	}
	if left.PublishedPort != right.PublishedPort {
		return left.PublishedPort < right.PublishedPort
	}
	return left.Protocol < right.Protocol
}

// Swap implements sort.Interface
func (p PortPublishers) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}

// ContainerProcSummary holds container processes top data
type ContainerProcSummary struct {
	ID        string
	Name      string
	Processes [][]string
	Titles    []string
}

// ImageSummary holds container image description
type ImageSummary struct {
	ID            string
	ContainerName string
	Repository    string
	Tag           string
	Size          int64
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
	Since      string
	Until      string
	Follow     bool
	Timestamps bool
}

// PauseOptions group options of the Pause API
type PauseOptions struct {
	// Services passed in the command line to be started
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
	Status(container, msg string)
	Register(container string)
}

// ContainerEventListener is a callback to process ContainerEvent from services
type ContainerEventListener func(event ContainerEvent)

// ContainerEvent notify an event has been collected on source container implementing Service
type ContainerEvent struct {
	Type      int
	Container string
	Service   string
	Line      string
	// ContainerEventExit only
	ExitCode   int
	Restarting bool
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
