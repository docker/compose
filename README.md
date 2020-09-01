# Docker Compose CLI

[![Actions Status](https://github.com/docker/compose-cli/workflows/Continuous%20integration/badge.svg)](https://github.com/docker/compose-cli/actions)

This CLI tool makes it easy to run containers in the cloud using either Amazon
Elastic Container Service
([ECS](https://aws.amazon.com/ecs))
or Microsoft Azure Container Instances
([ACI](https://azure.microsoft.com/services/container-instances))
using the Docker commands you already know.

To get started, all you need is:
* An [AWS](https://aws.amazon.com) or [Azure](https://azure.microsoft.com)
  account
* Windows: The Edge release of
  [Docker Desktop](https://hub.docker.com/editions/community/docker-ce-desktop-windows)
* macOS: The Edge release of
  [Docker Desktop](https://hub.docker.com/editions/community/docker-ce-desktop-mac)
* Linux:
  [Install script](INSTALL.md)

:warning: *This CLI is currently in beta please create*
*[issues](https://github.com/docker/compose-cli/issues) to leave feedback*

## Examples

* ECS: [Deploying Wordpress to the cloud](https://www.docker.com/blog/deploying-wordpress-to-the-cloud/)
* ACI: [Deploying a Minecraft server to the cloud](https://www.docker.com/blog/deploying-a-minecraft-docker-server-to-the-cloud/)

## Development

See the instructions in [BUILDING.md](BUILDING.md) for how to build the CLI and
run its tests; including the end to end tests for local containers, ACI, and
ECS.
The guide also includes instructions for releasing the CLI.

Before contributing, please read the [contribution guidelines](CONTRIBUTING.md)
which includes conventions used in this project.
