# AGENTS.md

This file provides guidance to AI coding agents when working with code in this repository.  
All agents need to conform to [AI Policies](./AI_POLICY.md)

## Project Overview

Docker Compose v2 is a tool for running multi-container applications on Docker, defined using the Compose file format. This is the Go implementation of Compose (the Python v1 version is legacy). It operates as both a Docker CLI plugin (`docker compose`) and a standalone binary.

## Build and Test Commands

### Building

```bash
# Build the CLI plugin (outputs to ./bin/build/docker-compose)
make build

# Cross-compile for all platforms (Windows, macOS, Linux)
make cross

# Install locally to ~/.docker/cli-plugins/docker-compose
make install

# Build using docker buildx
make binary
```

### Testing

```bash
# Run all unit tests
make test

# Run unit tests and update golden files
go test ./... -test.update-golden

# Run E2E tests in plugin mode (requires Docker Engine running)
make e2e-compose

# Run E2E tests in standalone mode
make e2e-compose-standalone

# Build and run E2E tests (both modes)
make build-and-e2e

# Run a specific E2E test
E2E_TEST=TestName make build-and-e2e-compose

# Exclude specific E2E tests
EXCLUDE_E2E_TESTS=TestName make build-and-e2e-compose
```

### Linting and Formatting

```bash
# Run linters
make lint

# Format code with gofumpt
make fmt

# Validate documentation is up-to-date
make validate-docs

# Validate license headers
make validate-headers

# Validate go.mod and go.sum
make validate-go-mod

# Run all validations
make validate
```

### Documentation

```bash
# Generate documentation
make docs

# Validate docs haven't changed
make validate-docs
```

### Other Commands

```bash
# Generate mocks (requires mockgen)
make mocks

# Check for dependency updates
make check-dependencies

# Clear builder cache
make cache-clear

# Run pre-commit checks (validation, linting, build, test, e2e)
make pre-commit
```

## Architecture Overview

### Core Execution Flow

1. **Entry Point** (`cmd/main.go`): The application can run as either a Docker CLI plugin or standalone binary. In standalone mode, it prepends "docker" to args and applies compatibility transformations.

2. **CLI Structure** (`cmd/compose/`): Uses Cobra for command structure. The `RootCommand` in `cmd/compose/compose.go` wires up all subcommands (up, down, build, etc.) and handles global flags.

3. **Compose Interface** (`pkg/api/api.go`): The core abstraction is the `Compose` interface which defines all Compose operations (Build, Up, Down, etc.). This is the contract that implementations must satisfy.

4. **Implementation** (`pkg/compose/`): The `composeService` struct in `pkg/compose/compose.go` implements the `Compose` interface. It wraps the Docker Engine API client and orchestrates container lifecycle operations.

### Key Architectural Components

**Compose API (`pkg/api/api.go`)**
- Defines the `Compose` interface with methods for all Compose operations
- Contains option structs (BuildOptions, UpOptions, DownOptions, etc.) that configure operations
- Defines data types for results (ContainerSummary, Stack, ImageSummary, etc.)

**Compose Service Implementation (`pkg/compose/`)**
- `compose.go`: Creates the service and provides core utilities
- `convergence.go`: Handles reconciliation logic to bring actual state to desired state
- `create.go`: Container creation and recreation logic
- `build.go`, `build_bake.go`, `build_classic.go`: Build implementations supporting BuildKit and classic builds
- `dependencies.go`: Dependency resolution and ordering
- Individual files for each command (up.go, down.go, ps.go, etc.)

**Command Handlers (`cmd/compose/`)**
- Each command has its own file (up.go, down.go, build.go, etc.)
- Commands parse flags, call `ProjectOptions.ToProject()` to load the Compose file, then invoke the Compose interface
- `compose.go` contains shared logic for project loading and option handling

**Project Loading**
- Uses `compose-spec/compose-go` library for parsing Compose files
- `ProjectOptions` struct handles project configuration (compose files, env files, profiles)
- `ToProject()` method loads and validates the project, resolves environment variables, applies profiles

**Progress/Output** (`pkg/progress/`)
- Unified progress reporting system supporting multiple output modes (TTY, plain, JSON, quiet)
- Mode is controlled by `--progress` flag and `COMPOSE_PROGRESS` environment variable

**Watch Mode** (`pkg/watch/`)
- File watching and syncing for development workflows
- Automatically rebuilds/restarts containers on file changes

### Important Patterns

**Dry Run Mode**: The service supports dry-run mode where operations are simulated but not executed. This is implemented via `api.DryRunClient` which wraps the Docker API client.

**Parallelism Control**: The `MaxConcurrency` method allows limiting concurrent operations against the Docker Engine API. This can be controlled via `COMPOSE_PARALLEL_LIMIT` environment variable or `--parallel` flag.

**Container Naming**: Containers are named using the pattern `{project}{separator}{service}{separator}{index}` where separator is "-" by default but "_" in compatibility mode.

**Labels**: Compose uses Docker labels to tag resources (containers, networks, volumes) with metadata:
- `com.docker.compose.project`: Project name
- `com.docker.compose.service`: Service name
- `com.docker.compose.version`: Compose version
- These labels are used to query and filter resources belonging to a project

**Project Discovery**: When no compose file is specified, Compose looks for `compose.yaml`, `compose.yml`, `docker-compose.yaml`, or `docker-compose.yml` in the current directory or parent directories.

## Development Notes

### Code Style

Follow standard Go conventions and the guidelines in CONTRIBUTING.md:
- All code must be formatted with `gofmt -s`
- All code should pass `golint` checks
- Document all declarations and methods, even private ones
- Variable names should be proportional to context - short names for small scopes
- No utils or helpers packages
- Tests should run with `go test` without external tooling

### Testing E2E Tests

E2E tests are in `pkg/e2e/` and use test fixtures from `pkg/e2e/fixtures/`. They require:
- A running Docker Engine
- The `docker-compose` binary built in `./bin/build/`
- Tests can run in either plugin mode or standalone mode

To run a single test: `E2E_TEST=TestBuild make build-and-e2e-compose`

### Building with Buildx

The project uses Docker Buildx for building and testing. The `docker-bake.hcl` file defines build targets. Key environment variables:
- `GO_VERSION`: Override Go version (defaults to version in Dockerfile)
- `BUILD_TAGS`: Build tags (default: "e2e")
- `DESTDIR`: Override output directory

### Compose File Loading

The project uses `github.com/compose-spec/compose-go/v2` for parsing Compose files. Key concepts:
- Project loading is highly configurable via `cli.ProjectOptions`
- Environment resolution happens in stages (OS env → .env files → compose file)
- Profiles filter which services are active
- Services can extend other services and include external Compose files

### Build Implementation

Compose supports multiple build backends:
- **BuildKit/Buildx** (default): Modern builder with advanced features, implemented in `build_bake.go`
- **Classic Builder**: Legacy builder for backward compatibility, implemented in `build_classic.go`
- Builder selection is automatic based on Docker Engine capabilities

### Remote Resources

Compose supports loading remote Compose files and build contexts from:
- Git repositories (`pkg/remote/git.go`)
- OCI registries (`pkg/remote/oci.go`)
- These are loaded via the `loader.ResourceLoader` interface

## Environment Variables

Key environment variables that affect behavior:
- `COMPOSE_PARALLEL_LIMIT`: Limit concurrent operations
- `COMPOSE_PROJECT_NAME`: Override project name
- `COMPOSE_FILE`: Specify compose file(s)
- `COMPOSE_PROFILES`: Specify active profiles
- `COMPOSE_COMPATIBILITY`: Enable v1 compatibility mode (uses "_" separator)
- `COMPOSE_REMOVE_ORPHANS`: Automatically remove orphaned containers
- `COMPOSE_IGNORE_ORPHANS`: Ignore orphaned containers
- `COMPOSE_ENV_FILES`: Default env files to load
- `COMPOSE_ANSI`: Control ANSI output (never/always/auto)
- `COMPOSE_PROGRESS`: Set progress output type
- `NO_COLOR`: Disable colored output
- `DOCKER_DEFAULT_PLATFORM`: Set default platform for builds

## Important Files

- `cmd/main.go`: Application entry point
- `cmd/compose/compose.go`: Root command and project loading
- `pkg/api/api.go`: Compose interface definition
- `pkg/compose/compose.go`: Compose service implementation (composeService)
- `pkg/compose/convergence.go`: Core reconciliation logic
- `Makefile`: Build and test automation
- `docker-bake.hcl`: Buildx configuration
- `go.mod`: Dependencies (requires Go 1.24+)

## AI-Assisted Contributions

If you're an AI coding agent (like Claude Code, GitHub Copilot, or similar) assisting a human developer:

### Required Reading
1. **Must read**: [AI_POLICY.md](AI_POLICY.md) - Disclosure requirements and quality standards
2. **Must follow**: All coding standards in this document
3. **Must ensure**: Human has tested the changes locally

### Common AI Pitfalls to Avoid

When working with Docker Compose:

❌ **Don't create unnecessary interfaces**: This project avoids over-abstraction. See CONTRIBUTING.md rule: "No utils or helpers packages"

❌ **Don't ignore existing patterns**: Before writing new code, search for similar functionality:
```bash
# Find existing patterns for similar operations
git grep "func.*Create.*Container" pkg/compose/
git grep "func.*Build.*Service" pkg/compose/
```

❌ **Don't skip test patterns**: Every package has `*_test.go` files. Match their style:
```bash
# Example: Before writing a test for create.go
cat pkg/compose/create_test.go | head -50
```

❌ **Don't ignore golangci-lint rules**: The project uses specific linter configurations in `.golangci.yml`:
- No `context.Background()` in tests (use `t.Context()`)
- No `context.TODO()` in tests (use `t.Context()`)
- No deprecated packages (io/ioutil, etc.)
- See `.golangci.yml` for full rules

### Good AI-Assisted Contribution Pattern

1. **Start with the issue**: Read the full GitHub issue and understand the user's actual problem
2. **Find existing patterns**: Search for similar code in the same package
3. **Follow the pattern**: Match structure, error handling, naming conventions
4. **Test comprehensively**: Run all relevant tests, not just unit tests
5. **Explain in human terms**: Write PR descriptions that explain WHY, not just WHAT

### Example: Good vs. Bad AI Contribution

**Bad**: AI generates code without context
```go
// pkg/compose/create.go
func CreateContainer(name string) error {
    // AI-generated code that "works" but doesn't follow patterns
    container := Container{Name: name}
    return container.Create()
}
```
**Why bad**: Doesn't match existing function signatures, error handling, or patterns in create.go

**Good**: AI generates code matching existing patterns
```go
// pkg/compose/create.go
// createContainer creates a container with the given configuration and returns its ID.
// It follows the project's standard error wrapping pattern and uses the existing
// composeService client for consistency with other container operations.
func (s *composeService) createContainer(ctx context.Context,
    project *types.Project,
    service types.ServiceConfig,
    opts api.CreateOptions) (string, error) {

    container, err := s.apiClient.ContainerCreate(ctx, ...)
    if err != nil {
        return "", fmt.Errorf("failed to create container for service %s: %w",
            service.Name, err)
    }
    return container.ID, nil
}
```
**Why good**: Matches signature patterns, uses existing types, follows error wrapping conventions, includes helpful comments

## Anti-Patterns Specific to Compose

Based on the codebase analysis, avoid these patterns:

### Anti-Pattern 1: Not Using compose-go Types
**Bad**: Defining your own types for things that exist in compose-spec/compose-go
**Good**: Import and use `github.com/compose-spec/compose-go/v2/types`

### Anti-Pattern 2: Ignoring the Compose Interface
**Bad**: Calling Docker API directly from command handlers
**Good**: Use the `api.Compose` interface defined in `pkg/api/api.go`

### Anti-Pattern 3: Not Following Container Naming Convention
**Bad**: `container_name := projectName + "_" + serviceName`
**Good**: Use `api.CreateContainerOptions` with proper project labels (see `pkg/compose/create.go`)

### Anti-Pattern 4: Inconsistent Error Messages
**Bad**: `return errors.New("error creating container")`
**Good**: `return fmt.Errorf("failed to create container for service %s: %w", serviceName, err)`

### Anti-Pattern 5: Missing Progress Reporting
**Bad**: Long operations with no output
**Good**: Use `pkg/progress` for operations that take time (build, pull, create)

### Anti-Pattern 6: Wrong Test Context
**Bad**: `ctx := context.Background()` in tests
**Good**: `ctx := t.Context()` (enforced by golangci-lint forbidigo rule)

## Testing Requirements for AI Agents

Before submitting, ensure:

```bash
# 1. Format all code
make fmt

# 2. Run linters (must pass)
make lint

# 3. Run unit tests (must pass)
make test

# 4. Run E2E tests for affected functionality
# For plugin mode (most common):
make build-and-e2e-compose

# For specific test:
E2E_TEST=TestUp make build-and-e2e-compose

# 5. Validate documentation is up to date
make validate-docs

# 6. Check all validations pass
make validate
```

If any of these fail, **do not submit the PR**. Fix the issues first.

## PR Description Template for AI-Assisted Contributions

```markdown
## Summary
[One paragraph explaining what this PR does and why]

## Related Issue
Fixes #[issue number]

## Changes
- [Specific change 1 with rationale]
- [Specific change 2 with rationale]
- [Specific change 3 with rationale]

## Testing
- [x] Unit tests pass: `make test`
- [x] E2E tests pass: `make build-and-e2e-compose`
- [x] Linting passes: `make lint`
- [x] Manually tested: [describe how]

## AI Tool Used
AI Tool: [e.g., Claude Code, GitHub Copilot]
Assistance Level: [Significant/Moderate/Minor]

## Maintainer Notes
[Anything reviewers should pay special attention to]
```
