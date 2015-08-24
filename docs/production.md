<!--[metadata]>
+++
title = "Using Compose in production"
description = "Guide to using Docker Compose in production"
keywords = ["documentation, docs,  docker, compose, orchestration, containers,  production"]
[menu.main]
parent="smn_workw_compose"
weight=1
+++
<![end-metadata]-->


## Using Compose in production

While **Compose is not yet considered production-ready**, if you'd like to experiment and learn more about using it in production deployments, this guide
can help.
The project is actively working towards becoming
production-ready; to learn more about the progress being made, check out the <a href="https://github.com/docker/compose/blob/master/ROADMAP.md">roadmap</a> for details
on how it's coming along and what still needs to be done.

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

For this reason, you'll probably want to define a separate Compose file, say
`production.yml`, which specifies production-appropriate configuration.

> **Note:** The [extends](extends.md) keyword is useful for maintaining multiple
> Compose files which re-use common services without having to manually copy and
> paste.

Once you've got an alternate configuration file, make Compose use it
by setting the `COMPOSE_FILE` environment variable:

    $ COMPOSE_FILE=production.yml
    $ docker-compose up -d

> **Note:** You can also use the file for a one-off command without setting
> an environment variable. You do this by passing the `-f` flag, e.g.,
> `docker-compose -f production.yml up -d`.

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
[Docker Machine](https://docs.docker.com/machine) makes managing local and
remote Docker hosts very easy, and is recommended even if you're not deploying
remotely.

Once you've set up your environment variables, all the normal `docker-compose`
commands will work with no further configuration.

### Running Compose on a Swarm cluster

[Docker Swarm](https://docs.docker.com/swarm), a Docker-native clustering
system, exposes the same API as a single Docker host, which means you can use
Compose against a Swarm instance and run your apps across multiple hosts.

Compose/Swarm integration is still in the experimental stage, and Swarm is still
in beta, but if you'd like to explore and experiment, check out the <a
href="https://github.com/docker/compose/blob/master/SWARM.md">integration
guide</a>.

## Compose documentation

- [Installing Compose](install.md)
- [Get started with Django](django.md)
- [Get started with Rails](rails.md)
- [Get started with Wordpress](wordpress.md)
- [Command line reference](/reference)
- [Yaml file reference](yml.md)
- [Compose environment variables](env.md)
- [Compose command line completion](completion.md)
