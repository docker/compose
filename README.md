Docker Compose
==============
[![Build Status](https://ci-next.docker.com/public/buildStatus/icon?job=compose/master)](https://ci-next.docker.com/public/job/compose/job/master/)

![Docker Compose](logo.png?raw=true "Docker Compose Logo")

** Compose V2 is on its way! :star_struck: **
---------------------------------------------

We are currently polishing the next generation of Docker Compose, to be released soon... :stopwatch: 
- Read more on [RC1 announcement](https://www.docker.com/blog/start-dev-environments-locally-compose-v2-rc-1-and-more-in-docker-desktop-3-6/). 
- Check the [v2 branch](https://github.com/docker/compose/tree/v2) on this repo.

We are working towards providing an easy way to install Compose V2 on Linux. Once this is available, Compose V2 will be marked as generally available, our current target for this is the end of October.

V1 vs V2 transition :hourglass_flowing_sand:
--------------------------------------------

"Generally Available" will mean:
- New features and bug fixes will only be considered in the V2 codebase 
- Users on Mac/Windows will be defaulted into Docker Compose V2, but can still opt out through the UI and the CLI. This means when running `docker-compose` you will actually be running `docker compose`
- Our current goal is for users on Linux to receive Compose v2 with the latest version of the docker CLI, but is pending some technical discussion. Users will be able to use [compose switch](https://github.com/docker/compose-switch) to enable redirection of `docker-compose` to `docker compose`
- Docker Compose V1 will continue to be maintained regarding security issues
- [v2 branch](https://github.com/docker/compose/tree/v2) will become the default one at that time

:lock_with_ink_pen: Depending on the feedback we receive from the community of GA and the adoption on Linux, we will come up with a plan to deprecate v1, but as of right now there is no concrete timeline as we want the transition to be as smooth as possible for all users. It is important to note that we have no plans of removing any aliasing of `docker-compose` to `docker compose`. We want to make it as easy as possible to switch and not break any ones scripts. We will follow up with a blog post in the next few months with more information of an exact timeline of V1 being marked as deprecated and end of support for security issues. We’d love to hear your feedback! You can provide it [here](https://github.com/docker/roadmap/issues/257).

About
-----

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
