# Table of Contents
- [Docker Compose v2](#docker-compose-v2)
- [About update and backward compatibility](#about-update-and-backward-compatibility)
- [Where to get Docker Compose](#where-to-get-docker-compose)
    + [Windows and macOS](#windows-and-macos)
    + [Linux](#linux)
- [Quick Start](#quick-start)
- [Contributing](#contributing)
# Docker Compose v2
<p align="center">
    <a href="https://github.com/docker/compose/releases/latest"><img src="https://img.shields.io/github/release/docker/compose.svg?style=flat-square" alt="GitHub release"></a>
    <a href="https://pkg.go.dev/github.com/docker/compose/v2"><img src="https://img.shields.io/badge/go.dev-docs-007d9c?style=flat-square&logo=go&logoColor=white" alt="PkgGoDev"></a>
    <a href="https://github.com/docker/compose/actions?query=workflow%3Aci"><img src="https://img.shields.io/static/v1?label=ci&message=actions&color=green&logo=github" alt="Build Status"></a>
    <a href="https://goreportcard.com/report/github.com/docker/compose/v2"><img src="https://goreportcard.com/badge/github.com/docker/compose/v2?style=flat-square" alt="Go Report Card"></a>
    <a href="https://codecov.io/gh/docker/compose"><img src="https://codecov.io/gh/docker/compose/branch/master/graph/badge.svg?token=HP3K4Y4ctu" alt="Codecov"></a>
    <a href="https://api.securityscorecards.dev/projects/github.com/docker/compose"><img src="https://api.securityscorecards.dev/projects/github.com/docker/compose/badge" alt="OpenSSF Scorecard"></a>
</p>
<hr>
<p align="center">
    <img src="logo.png?raw=true" alt="Docker Compose">
</p>

Docker Compose is a tool for running multi-container applications on Docker
defined using the [Compose file format](https://compose-spec.io).
A Compose file is used to define how one or more containers that make up
your application are configured.
Once you have a Compose file, you can create and start your application with a
single command: `docker compose up`.

# About update and backward compatibility

Docker Compose V2 is a major version bump release of Docker Compose. It has been completely rewritten from scratch in Golang (V1 was in Python). The installation instructions for Compose V2 differ from V1. V2 is not a standalone binary anymore, and installation scripts will have to be adjusted. Some commands are different.

For a smooth transition from legacy docker-compose 1.xx, please consider installing [compose-switch](https://github.com/docker/compose-switch) to translate `docker-compose ...` commands into Compose V2's `docker compose .... `. Also check V2's `--compatibility` flag.

# Where to get Docker Compose

### Windows and macOS

Docker Compose is included in
[Docker Desktop](https://www.docker.com/products/docker-desktop)
for Windows and macOS.

### Linux

You can download Docker Compose binaries from the
[release page](https://github.com/docker/compose/releases) on this repository.

Rename the relevant binary for your OS to `docker-compose` and copy it to `$HOME/.docker/cli-plugins`

Or copy it into one of these folders to install it system-wide:

* `/usr/local/lib/docker/cli-plugins` OR `/usr/local/libexec/docker/cli-plugins`
* `/usr/lib/docker/cli-plugins` OR `/usr/libexec/docker/cli-plugins`

(might require making the downloaded file executable with `chmod +x`)


Quick Start
-----------

Using Docker Compose is a three-step process:
1. Define your app's environment with a `Dockerfile` so it can be
   reproduced anywhere.
2. Define the services that make up your app in `docker-compose.yml` so
   they can be run together in an isolated environment.
3. Lastly, run `docker compose up` and Compose will start and run your entire
   app.

A Compose file looks like this:

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

Contributing
------------

Want to help develop Docker Compose? Check out our
[contributing documentation](CONTRIBUTING.md).

If you find an issue, please report it on the
[issue tracker](https://github.com/docker/compose/issues/new/choose).
