Docker Compose
==============
[![Build Status](https://ci-next.docker.com/public/buildStatus/icon?job=compose/master)](https://ci-next.docker.com/public/job/compose/job/master/)

![Docker Compose](logo.png?raw=true "Docker Compose Logo")

Docker Compose is a tool for running multi-container applications on Docker
defined using the [Compose file format](https://compose-spec.io).
A Compose file is used to define how the one or more containers that make up
your application are configured.
Once you have a Compose file, you can create and start your application with a
single command: `docker-compose up`.

Compose files can be used to deploy applications locally, or to the cloud on
[Amazon ECS](https://aws.amazon.com/ecs) or
[Microsoft ACI](https://azure.microsoft.com/services/container-instances/) using
the Docker CLI. You can read more about how to do this:
- [Compose for Amazon ECS](https://docs.docker.com/engine/context/ecs-integration/)
- [Compose for Microsoft ACI](https://docs.docker.com/engine/context/aci-integration/)

Where to get Docker Compose
----------------------------

### Windows and macOS

Docker Compose is included in
[Docker Desktop](https://www.docker.com/products/docker-desktop)
for Windows and macOS.

### Linux

You can download Docker Compose binaries from the
[release page](https://github.com/docker/compose/releases) on this repository.

### Using pip

If your platform is not supported, you can download Docker Compose using `pip`:

```console
pip install docker-compose
```

> **Note:** Docker Compose requires Python 3.6 or later.

Quick Start
-----------

Using Docker Compose is basically a three-step process:
1. Define your app's environment with a `Dockerfile` so it can be
   reproduced anywhere.
2. Define the services that make up your app in `docker-compose.yml` so
   they can be run together in an isolated environment.
3. Lastly, run `docker-compose up` and Compose will start and run your entire
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

You can find examples of Compose applications in our
[Awesome Compose repository](https://github.com/docker/awesome-compose).

For more information about the Compose format, see the
[Compose file reference](https://docs.docker.com/compose/compose-file/).

Contributing
------------

Want to help develop Docker Compose? Check out our
[contributing documentation](https://github.com/docker/compose/blob/master/CONTRIBUTING.md).

If you find an issue, please report it on the
[issue tracker](https://github.com/docker/compose/issues/new/choose).

Releasing
---------

Releases are built by maintainers, following an outline of the [release process](https://github.com/docker/compose/blob/master/project/RELEASE-PROCESS.md).
