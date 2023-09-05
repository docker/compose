# Comprehensive Guide to Docker Compose v2

Welcome to the comprehensive guide for Docker Compose v2. In this guide, we'll cover everything you need to know about Docker Compose, from installation to usage and contributing to the project. Here's what you'll find in this guide:

## Table of Contents

- [Docker Compose v2](#docker-compose-v2)
- [Where to get Docker Compose](#where-to-get-docker-compose)
    - [Windows and macOS](#windows-and-macos)
    - [Linux](#linux)
- [Quick Start](#quick-start)
- [Contributing](#contributing)
- [Legacy](#legacy)
---

## Docker Compose v2

![GitHub release](https://img.shields.io/github/release/docker/compose.svg?style=flat-square)
![PkgGoDev](https://img.shields.io/badge/go.dev-docs-007d9c?style=flat-square&logo=go&logoColor=white)
![Build Status](https://img.shields.io/github/actions/workflow/status/docker/compose/ci.yml?label=ci&logo=github&style=flat-square)
![Go Report Card](https://goreportcard.com/badge/github.com/docker/compose/v2?style=flat-square)
![Codecov](https://codecov.io/gh/docker/compose/branch/main/graph/badge.svg?token=HP3K4Y4ctu)
![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/docker/compose/badge)
![Docker Compose Logo](logo.png?raw=true)

Docker Compose is a powerful tool designed for running multi-container applications on Docker. It relies on the [Compose file format](https://compose-spec.io) to define the configuration of one or more containers that make up your application. With a Compose file in hand, you can effortlessly create and start your application using a single command: `docker compose up`.

## Where to get Docker Compose

### Windows and macOS

If you're using Windows or macOS, you can conveniently obtain Docker Compose as it's bundled with [Docker Desktop](https://www.docker.com/products/docker-desktop).

### Linux

For Linux users, Docker Compose binaries are available for download on the [release page](https://github.com/docker/compose/releases) of the official Docker Compose repository. Follow these steps to install it:

1. Download the relevant binary for your operating system.
2. Rename the binary to `docker-compose`.
3. Copy it to `$HOME/.docker/cli-plugins` for user-specific installation.
   OR
   Install it system-wide by copying it to one of these folders:
    - `/usr/local/lib/docker/cli-plugins` OR `/usr/local/libexec/docker/cli-plugins`
    - `/usr/lib/docker/cli-plugins` OR `/usr/libexec/docker/cli-plugins`
      (You might need to make the downloaded file executable with `chmod +x`).

---

## Quick Start

Using Docker Compose is a straightforward three-step process:

1. Define your application's environment by creating a `Dockerfile` so that it can be reproduced consistently across different systems.
2. Specify the services that constitute your application in a `docker-compose.yml` file, enabling them to run together in an isolated environment.
3. Finally, execute `docker compose up`, and Docker Compose will initiate and run your entire application.

Here's an example of a Compose file:

```yaml
services:
  web:
    build: .
    ports:
      - "5000:5000"
    volumes:
      - .:/code
  redis:
    image: redis
```

## Contributing

If you're interested in contributing to the development of Docker Compose, we welcome your contributions. Please refer to our [contributing documentation](CONTRIBUTING.md) for detailed instructions on how to get started.
If you encounter any issues or have suggestions, please report them on the [issue tracker](https://github.com/docker/compose/issues/new/choose).


## Legacy

For users interested in the Python version of Compose, it is available under the `v1` [branch](https://github.com/docker/compose/tree/v1). Please note that Docker Compose v2 is the recommended and actively maintained version, offering numerous improvements and features.
