<!--[metadata]>
+++
title = "Using Compose in production"
description = "Guide to using Docker Compose in production"
keywords = ["documentation, docs,  docker, compose, orchestration, containers,  production"]
[menu.main]
parent="workw_compose"
weight=1
+++
<![end-metadata]-->


## Using Compose in production

> Compose is still primarily aimed at development and testing environments.
> Compose may be used for smaller production deployments, but is probably
> not yet suitable for larger deployments.

When deploying to production, you'll almost certainly want to make changes to
your app configuration that are more appropriate to a live environment. These
changes may include:

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
[Docker Machine](https://docs.docker.com/machine/) makes managing local and
remote Docker hosts very easy, and is recommended even if you're not deploying
remotely.

Once you've set up your environment variables, all the normal `docker-compose`
commands will work with no further configuration.

### Running Compose on a Swarm cluster

[Docker Swarm](https://docs.docker.com/swarm/), a Docker-native clustering
system, exposes the same API as a single Docker host, which means you can use
Compose against a Swarm instance and run your apps across multiple hosts.

Compose/Swarm integration is still in the experimental stage, and Swarm is still
in beta, but if you'd like to explore and experiment, check out the <a
href="https://github.com/docker/compose/blob/master/SWARM.md">integration
guide</a>.

## Compose documentation

- [Installing Compose](install.md)
- [Command line reference](./reference/index.md)
- [Compose file reference](compose-file.md)
