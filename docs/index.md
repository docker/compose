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

Compose is a tool for defining and running multi-container applications with
Docker. With Compose, you define a multi-container application in a single
file, then spin your application up in a single command which does everything
that needs to be done to get it running.

Compose is great for development environments, staging servers, and CI. We don't
recommend that you use it in production yet.

Using Compose is basically a three-step process.

1. Define your app's environment with a `Dockerfile` so it can be
reproduced anywhere.
2. Define the services that make up your app in `docker-compose.yml` so
they can be run together in an isolated environment:
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


## Release Notes

To see a detailed list of changes for past and current releases of Docker
Compose, please refer to the [CHANGELOG](https://github.com/docker/compose/blob/master/CHANGELOG.md).

## Getting help

Docker Compose is under active development. If you need help, would like to
contribute, or simply want to talk about the project with like-minded
individuals, we have a number of open channels for communication.

* To report bugs or file feature requests: please use the [issue tracker on Github](https://github.com/docker/compose/issues).

* To talk about the project with people in real time: please join the
  `#docker-compose` channel on freenode IRC.

* To contribute code or documentation changes: please submit a [pull request on Github](https://github.com/docker/compose/pulls).

For more information and resources, please visit the [Getting Help project page](https://docs.docker.com/project/get-help/).
