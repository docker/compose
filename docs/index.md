<!--[metadata]>
+++
title = "Overview of Docker Compose"
description = "Introduction and Overview of Compose"
keywords = ["documentation, docs,  docker, compose, orchestration,  containers"]
[menu.main]
parent="smn_workw_compose"
+++
<![end-metadata]-->


# Overview of Docker Compose

Compose is a tool for defining and running multi-container Docker applications.
With Compose, you define a multi-container application in a compose
file then, using a single command, you create and start all the containers
from your configuration. To learn more about all the features of Compose
see [the list of features](#features)

Compose is great for development, testing, and staging environments, as well as
CI workflows. You can learn more about each case in
[Common Use Cases](#common-use-cases).

Using Compose is basically a three-step process.

1. Define your app's environment with a `Dockerfile` so it can be
reproduced anywhere.
2. Define the services that make up your app in `docker-compose.yml` so
they can be run together in an isolated environment.
3. Lastly, run `docker-compose up` and Compose will start and run your entire app.

A `docker-compose.yml` looks like this:

    web:
      build: .
      ports:
       - "5000:5000"
      volumes:
       - .:/code
      links:
       - redis
    redis:
      image: redis

For more information about the Compose file, see the
[Compose file reference](yml.md)

Compose has commands for managing the whole lifecycle of your application:

 * Start, stop and rebuild services
 * View the status of running services
 * Stream the log output of running services
 * Run a one-off command on a service

## Compose documentation

- [Installing Compose](install.md)
- [Getting Started](gettingstarted.md)
- [Get started with Django](django.md)
- [Get started with Rails](rails.md)
- [Get started with WordPress](wordpress.md)
- [Command line reference](./reference/index.md)
- [Compose file reference](compose-file.md)

## Features

#### Preserve volume data

Compose preserves all volumes used by your services. When `docker-compose up`
runs, if it finds any containers from previous runs, it copies the volumes from
the old container to the new container. This process ensures that any data
you've created in volumes isn't lost.


#### Only recreate containers that have changed

Compose caches the configuration used to create a container. When you
restart a service that has not changed, Compose re-uses the existing
containers. Re-using containers means that you can make changes to your
environment very quickly.


#### Variables and moving a composition to different environments

> New in `docker-compose` 1.5

Compose supports variables in the Compose file. You can use these variables
to customize your composition for different environments, or different users.
See [Variable substitution](compose-file.md#variable-substitution) for more
details.

Compose files can also be extended from other files using the `extends`
field in a compose file, or by using multiple files. See [extends](extends.md)
for more details.


## Common Use Cases

Compose can be used in many different ways. Some common use cases are outlined
below.

### Development environments

When you're developing software it is often helpful to be able to run the
application and interact with it. If the application has any service dependencies
(databases, queues, caches, web services, etc) you need a way to document the
dependencies, configuration and operation of each. Compose provides a convenient
format for definition these dependencies (the [Compose file](yml.md)) and a CLI
tool for starting an isolated environment. Compose can replace a multi-page
"developer getting started guide" with a single machine readable configuration
file and a single command `docker-compose up`.

### Automated testing environments

An important part of any Continuous Deployment or Continuous Integration process
is the automated test suite. Automated end-to-end testing requires an
environment in which to run tests. Compose provides a convenient way to create
and destroy isolated testing environments for your test suite. By defining the full
environment in a [Compose file](yml.md) you can create and destroy these
environments in just a few commands:

    $ docker-compose up -d
    $ ./run_tests
    $ docker-compose stop
    $ docker-compose rm -f

### Single host deployments

Compose has traditionally been focused on development and testing workflows,
but with each release we're making progress on more production-oriented features.
Compose can be used to deploy to a remote docker engine, for example a cloud
instance provisioned with [Docker Machine](https://docs.docker.com/machine/) or
a [Docker Swarm](https://docs.docker.com/swarm/) cluster.

See [compose in production](production.md) for more details.


## Release Notes

To see a detailed list of changes for past and current releases of Docker
Compose, please refer to the
[CHANGELOG](https://github.com/docker/compose/blob/master/CHANGELOG.md).

## Getting help

Docker Compose is under active development. If you need help, would like to
contribute, or simply want to talk about the project with like-minded
individuals, we have a number of open channels for communication.

* To report bugs or file feature requests: please use the [issue tracker on Github](https://github.com/docker/compose/issues).

* To talk about the project with people in real time: please join the
  `#docker-compose` channel on freenode IRC.

* To contribute code or documentation changes: please submit a [pull request on Github](https://github.com/docker/compose/pulls).

For more information and resources, please visit the [Getting Help project page](https://docs.docker.com/project/get-help/).
