# Using the `docker/compose` SDK

The `docker/compose` package can be used as a Go library by third-party applications to programmatically manage
containerized applications defined in Compose files. This SDK provides a comprehensive API that lets you
integrate Compose functionality directly into your applications, allowing you to load, validate, and manage
multi-container environments without relying on the Compose CLI. 

Whether you need to orchestrate containers as part of
a deployment pipeline, build custom management tools, or embed container orchestration into your application, the
Compose SDK offers the same powerful capabilities that drive the Docker Compose command-line tool.

## Set up the SDK

To get started, create an SDK instance using the `NewComposeService()` function, which initializes a service with the
necessary configuration to interact with the Docker daemon and manage Compose projects. This service instance provides
methods for all core Compose operations including creating, starting, stopping, and removing containers, as well as
loading and validating Compose files. The service handles the underlying Docker API interactions and resource
management, allowing you to focus on your application logic.

## Example usage

Here's a basic example demonstrating how to load a Compose project and start the services:

```go
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
```

This example demonstrates the core workflow - creating a service instance, loading a project from a Compose file, and
starting the services. The SDK provides many additional operations for managing the lifecycle of your containerized
application.

## Customizing the SDK

The `NewComposeService()` function accepts optional `compose.Option` parameters to customize the SDK behavior. These
options allow you to configure I/O streams, concurrency limits, dry-run mode, and other advanced features.

```go
    // Create a custom output buffer to capture logs
    var outputBuffer bytes.Buffer

    // Create a compose service with custom options
    service, err := compose.NewComposeService(dockerCLI,
        compose.WithOutputStream(&outputBuffer),          // Redirect output to custom writer
        compose.WithErrorStream(os.Stderr),               // Use stderr for errors
        compose.WithMaxConcurrency(4),                    // Limit concurrent operations
        compose.WithPrompt(compose.AlwaysOkPrompt()),     // Auto-confirm all prompts
    )
```

### Available options

- `WithOutputStream(io.Writer)` - Redirect standard output to a custom writer
- `WithErrorStream(io.Writer)` - Redirect error output to a custom writer
- `WithInputStream(io.Reader)` - Provide a custom input stream for interactive prompts
- `WithStreams(out, err, in)` - Set all I/O streams at once
- `WithMaxConcurrency(int)` - Limit the number of concurrent operations against the Docker API
- `WithPrompt(Prompt)` - Customize user confirmation behavior (use `AlwaysOkPrompt()` for non-interactive mode)
- `WithDryRun` - Run operations in dry-run mode without actually applying changes
- `WithContextInfo(api.ContextInfo)` - Set custom Docker context information
- `WithProxyConfig(map[string]string)` - Configure HTTP proxy settings for builds
- `WithEventProcessor(progress.EventProcessor)` - Receive progress events and operation notifications

These options provide fine-grained control over the SDK's behavior, making it suitable for various integration
scenarios including CLI tools, web services, automation scripts, and testing environments.

## Tracking operations with `EventProcessor`

The `EventProcessor` interface allows you to monitor Compose operations in real-time by receiving events about changes
applied to Docker resources such as images, containers, volumes, and networks. This is particularly useful for building
user interfaces, logging systems, or monitoring tools that need to track the progress of Compose operations.

### Understanding `EventProcessor`

A Compose operation, such as `up`, `down`, `build`, performs a series of changes to Docker resources. The
`EventProcessor` receives notifications about these changes through three key methods:

- `Start(ctx, operation)` - Called when a Compose operation begins, for example `up`
- `On(events...)` - Called with progress events for individual resource changes, for example, container starting, image
  being pulled
- `Done(operation, success)` - Called when the operation completes, indicating success or failure

Each event contains information about the resource being modified, its current status, and progress indicators when
applicable (such as download progress for image pulls).

### Event status types

Events report resource changes with the following status types:

- Working - Operation is in progress, for example, creating, starting, pulling
- Done - Operation completed successfully
- Warning - Operation completed with warnings
- Error - Operation failed

Common status text values include: `Creating`, `Created`, `Starting`, `Started`, `Running`, `Stopping`, `Stopped`,
`Removing`, `Removed`, `Building`, `Built`, `Pulling`, `Pulled`, and more.

### Built-in `EventProcessor` implementations

The SDK provides three ready-to-use `EventProcessor` implementations:

- `progress.NewTTYWriter(io.Writer)` - Renders an interactive terminal UI with progress bars and task lists
  (similar to the Docker Compose CLI output)
- `progress.NewPlainWriter(io.Writer)` - Outputs simple text-based progress messages suitable for non-interactive
  environments or log files
- `progress.NewJSONWriter()` - Render events as JSON objects
- `progress.NewQuietWriter()` - (Default) Silently processes events without producing any output

Using `EventProcessor`, a custom UI can be plugged into `docker/compose`.
