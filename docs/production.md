<!--[metadata]>
+++
title = "Using Compose in Production"
description = "Guide to using Docker Compose in production"
keywords = ["documentation, docs,  docker, compose, orchestration, containers,  production"]
[menu.main]
parent="workw_compose"
weight=22
+++
<![end-metadata]-->


## Using Compose in production

When you define your app with Compose in development, you can use this
definition to run your application in different environments such as CI,
staging, and production.

The easiest way to deploy an application is to run it on a single server,
similar to how you would run your development environment. If you want to scale
up your application, you can run Compose apps on a Swarm cluster.

### Modify your Compose file for production

You'll almost certainly want to make changes to your app configuration that are
more appropriate to a live environment. These changes may include:

- Removing any volume bindings for application code, so that code stays inside
  the container and can't be changed from outside
- Binding to different ports on the host
- Setting environment variables differently (e.g., to decrease the verbosity of
  logging, or to enable email sending)
- Specifying a restart policy (e.g., `restart: always`) to avoid downtime
- Adding extra services (e.g., a log aggregator)

For this reason, you'll probably want to define an additional Compose file, say
`production.yml`, which specifies production-appropriate
configuration. This configuration file only needs to include the changes you'd
like to make from the original Compose file.  The additional Compose file
can be applied over the original `docker-compose.yml` to create a new configuration.

Once you've got a second configuration file, tell Compose to use it with the
`-f` option:

    $ docker-compose -f docker-compose.yml -f production.yml up -d

See [Using multiple compose files](extends.md#different-environments) for a more
complete example.

### Deploying changes

When you make changes to your app code, you'll need to rebuild your image and
recreate your app's containers. To redeploy a service called
`web`, you would use:

    $ docker-compose build web
    $ docker-compose up --no-deps -d web

This will first rebuild the image for `web` and then stop, destroy, and recreate
*just* the `web` service. The `--no-deps` flag prevents Compose from also
recreating any services which `web` depends on.

### Running Compose on a single server

You can use Compose to deploy an app to a remote Docker host by setting the
`DOCKER_HOST`, `DOCKER_TLS_VERIFY`, and `DOCKER_CERT_PATH` environment variables
appropriately. For tasks like this,
[Docker Machine](/machine/overview.md) makes managing local and
remote Docker hosts very easy, and is recommended even if you're not deploying
remotely.

Once you've set up your environment variables, all the normal `docker-compose`
commands will work with no further configuration.

### Running Compose on a Swarm cluster

[Docker Swarm](/swarm/overview.md), a Docker-native clustering
system, exposes the same API as a single Docker host, which means you can use
Compose against a Swarm instance and run your apps across multiple hosts.

Read more about the Compose/Swarm integration in the
[integration guide](swarm.md).

## Compose documentation

- [Installing Compose](install.md)
- [Command line reference](./reference/index.md)
- [Compose file reference](compose-file.md)
