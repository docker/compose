page_title: Using Compose in production
page_description: Guide to using Docker Compose in production
page_keywords: documentation, docs,  docker, compose, orchestration, containers, production


## Using Compose in production

While **Compose is not yet considered production-ready**, you can try using it
for production deployments if you're feeling brave. Production-readiness is an
active, ongoing project - see the
[roadmap](https://github.com/docker/compose/blob/master/ROADMAP.md) for details
on how it's coming along and what needs to be done.

When deploying to production, you'll almost certainly want to make changes to
your app configuration that are more appropriate to a live environment. This may
include:

- Removing any volume bindings for application code, so that code stays inside
  the container and can't be changed from outside
- Binding to different ports on the host
- Setting environment variables differently (e.g. to decrease the verbosity of
  logging, or to enable email sending)
- Specifying a restart policy (e.g. `restart: always`) to avoid downtime
- Adding extra services (e.g. a log aggregator)

For this reason, you'll probably want to define a separate Compose file, say
`production.yml`, which specifies production-appropriate configuration.

<!-- TODO: uncomment when the `extends` guide is merged
> **Note:** The [extends](extends.md) keyword is useful for maintaining multiple
> Compose files which re-use common services without having to manually copy and
> paste.
-->

Once you've got an alternate configuration file, you can make Compose use it
by setting the `COMPOSE_FILE` environment variable:

    $ COMPOSE_FILE=production.yml
    $ docker-compose up -d

> **Note:** You can also use the file for a one-off command without setting
> an environment variable by passing the `-f` flag, e.g.
> `docker-compose -f production.yml up -d`.

### Deploying changes

When you make changes to your app code, you'll need to rebuild your image and
recreate your app containers. If the service you want to redeploy is called
`web`, this will look like:

    $ docker-compose build web
    $ docker-compose up --no-deps -d web

This will first rebuild the image for `web` and then stop, destroy and recreate
*just* the `web` service. The `--no-deps` flag prevents Compose from also
recreating any services which `web` depends on.

### Run Compose on a single server

You can use Compose to deploy an app to a remote Docker host by setting the
`DOCKER_HOST`, `DOCKER_TLS_VERIFY` and `DOCKER_CERT_PATH` environment variables
appropriately. [Docker Machine](https://docs.docker.com/machine) makes managing
local and remote Docker hosts very easy, and is recommended even if you're not
deploying remotely.

Once you've set up your environment variables, all the normal `docker-compose`
commands will work with no extra configuration.

### Run Compose on a Swarm cluster

[Docker Swarm](https://docs.docker.com/swarm), a Docker-native clustering
system, exposes the same API as a single Docker host, which means you can use
Compose against a Swarm instance and run your apps across multiple hosts.

Compose/Swarm integration is still in the experimental stage, and Swarm is still
in beta, but if you're interested to try it out, check out the
[integration guide](https://github.com/docker/compose/blob/master/SWARM.md).
