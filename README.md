# Docker Compose CLI

[![Actions Status](https://github.com/docker/compose-cli/workflows/Continuous%20integration/badge.svg)](https://github.com/docker/compose-cli/actions)
[![Actions Status](https://github.com/docker/compose-cli/workflows/Windows%20CI/badge.svg)](https://github.com/docker/compose-cli/actions)

This Compose CLI tool makes it easy to run Docker containers and Docker Compose applications:
* locally as a command in the docker CLI, using `docker compose ...` comands.
* in the cloud using either Amazon Elastic Container Service
([ECS](https://aws.amazon.com/ecs))
or Microsoft Azure Container Instances
([ACI](https://azure.microsoft.com/services/container-instances))
using the Docker commands you already know.
  
**Note: Compose CLI is released under the 1.x tag, until "Compose v2" gets a new home**

## Compose v2 (a.k.a "Local Docker Compose")

The `docker compose` local command is the next major version for docker-compose, and it supports the same commands and flags, in order to be used as a drop-in replacement.
[Here](https://github.com/docker/compose-cli/issues/1283) is a checklist of docker-compose commands and flags that are implemented in `docker compose`.

This `docker compose` local command :
* has a better integration with the rest of the docker ecosystem (being written in go, it's easier to share functionality with the Docker CLI and other Docker libraries)
* is quicker and uses more parallelism to run multiple tasks in parallel. It also uses buildkit by default
* includes additional commands, like `docker compose ls` to list current compose projects

**Note: Compose v2 is released under the 2.x tag, until "Compose v2" gets a new home**

Compose v2 can be installed manually as a CLI plugin, by downloading latest v2.x release from https://github.com/docker/compose-cli/releases for your architecture and move into `~/.docker/cli-plugins/docker-compose`

## Getting started

To get started with Compose CLI, all you need is:

* Windows: The current release of
  [Docker Desktop](https://hub.docker.com/editions/community/docker-ce-desktop-windows)
* macOS: The current release of
  [Docker Desktop](https://hub.docker.com/editions/community/docker-ce-desktop-mac)
* Linux:
  [Install script](INSTALL.md)
* An [AWS](https://aws.amazon.com) or [Azure](https://azure.microsoft.com)
  account in order to use the Compose Cloud integration

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
