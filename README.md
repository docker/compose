# Docker Compose CLI

[![Actions Status](https://github.com/docker/compose-cli/workflows/Continuous%20integration/badge.svg)](https://github.com/docker/compose-cli/actions)
[![Actions Status](https://github.com/docker/compose-cli/workflows/Windows%20CI/badge.svg)](https://github.com/docker/compose-cli/actions)

This CLI tool makes it easy to run Docker containers and Docker Compose applications in the cloud using either Amazon
Elastic Container Service
([ECS](https://aws.amazon.com/ecs))
or Microsoft Azure Container Instances
([ACI](https://azure.microsoft.com/services/container-instances))
using the Docker commands you already know.

To get started, all you need is:

* An [AWS](https://aws.amazon.com) or [Azure](https://azure.microsoft.com)
  account
* Windows: The Stable or Edge release of
  [Docker Desktop](https://hub.docker.com/editions/community/docker-ce-desktop-windows)
* macOS: The Stable or Edge release of
  [Docker Desktop](https://hub.docker.com/editions/community/docker-ce-desktop-mac)
* Linux:
  [Install script](INSTALL.md)

Please create [issues](https://github.com/docker/compose-cli/issues) to leave feedback.

## Examples

* ECS: [Deploying Wordpress to the cloud](https://www.docker.com/blog/deploying-wordpress-to-the-cloud/)
* ACI: [Deploying a Minecraft server to the cloud](https://www.docker.com/blog/deploying-a-minecraft-docker-server-to-the-cloud/)
* ACI: [Setting Up Cloud Deployments Using Docker, Azure and Github Actions](https://www.docker.com/blog/setting-up-cloud-deployments-using-docker-azure-and-github-actions/)

## Development

See the instructions in [BUILDING.md](BUILDING.md) for how to build the CLI and
run its tests; including the end to end tests for local containers, ACI, and
ECS.
The guide also includes instructions for releasing the CLI.

Before contributing, please read the [contribution guidelines](CONTRIBUTING.md)
which includes conventions used in this project.
