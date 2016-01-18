<!--[metadata]>
+++
title = "Networking in Compose"
description = "How Compose sets up networking between containers"
keywords = ["documentation, docs,  docker, compose, orchestration, containers, networking"]
[menu.main]
parent="smn_workw_compose"
weight=6
+++
<![end-metadata]-->


# Networking in Compose

> **Note:** This document only applies if you're using v2 of the [Compose file format](compose-file.md). Networking features are not supported for legacy Compose files.

By default Compose sets up a single
[network](/engine/reference/commandline/network_create.md) for your app. Each
container for a service joins the default network and is both *reachable* by
other containers on that network, and *discoverable* by them at a hostname
identical to the container name.

> **Note:** Your app's network is given a name based on the "project name", which is based on the name of the directory it lives in. You can override the project name with either the [`--project-name` flag](reference/docker-compose.md) or the [`COMPOSE_PROJECT_NAME` environment variable](reference/overview.md#compose-project-name).

For example, suppose your app is in a directory called `myapp`, and your `docker-compose.yml` looks like this:

    version: 2

    services:
      web:
        build: .
        ports:
          - "8000:8000"
      db:
        image: postgres

When you run `docker-compose up`, the following happens:

1.  A network called `myapp_default` is created.
2.  A container is created using `web`'s configuration. It joins the network
    `myapp_default` under the name `web`.
3.  A container is created using `db`'s configuration. It joins the network
    `myapp_default` under the name `db`.

Each container can now look up the hostname `web` or `db` and
get back the appropriate container's IP address. For example, `web`'s
application code could connect to the URL `postgres://db:5432` and start
using the Postgres database.

Because `web` explicitly maps a port, it's also accessible from the outside world via port 8000 on your Docker host's network interface.

## Updating containers

If you make a configuration change to a service and run `docker-compose up` to update it, the old container will be removed and the new one will join the network under a different IP address but the same name. Running containers will be able to look up that name and connect to the new address, but the old address will stop working.

If any containers have connections open to the old container, they will be closed. It is a container's responsibility to detect this condition, look up the name again and reconnect.

## Links

Docker links are a one-way, single-host communication system. They should now be considered deprecated, and as part of upgrading your app to the v2 format, you must remove any `links` sections from your `docker-compose.yml` and use service names (e.g. `web`, `db`) as the hostnames to connect to.

## Multi-host networking

When deploying a Compose application to a Swarm cluster, you can make use of the built-in `overlay` driver to enable multi-host communication between containers with no changes to application code. Consult the [Getting started with multi-host networking](/engine/userguide/networking/get-started-overlay.md) to see how to set up the overlay driver, and then specify `driver: overlay` in your networking config (see the sections below for how to do this).

## Specifying custom networks

Instead of just using the default app network, you can specify your own networks with the top-level `networks` key. This lets you create more complex topologies and specify [custom network drivers](/engine/extend/plugins_network.md) and options. You can also use it to connect services to externally-created networks which aren't managed by Compose.

Each service can specify what networks to connect to with the *service-level* `networks` key, which is a list of names referencing entries under the *top-level* `networks` key.

Here's an example Compose file defining several networks. The `proxy` service is the gateway to the outside world, via a network called `outside` which is expected to already exist. `proxy` is isolated from the `db` service, because they do not share a network in common - only `app` can talk to both.

    version: 2

    services:
      proxy:
        build: ./proxy
        networks:
          - outside
          - front
      app:
        build: ./app
        networks:
          - front
          - back
      db:
        image: postgres
        networks:
          - back

    networks:
      front:
        # Use the overlay driver for multi-host communication
        driver: overlay
      back:
        # Use a custom driver which takes special options
        driver: my-custom-driver
        options:
          foo: "1"
          bar: "2"
      outside:
        # The 'outside' network is expected to already exist - Compose will not
        # attempt to create it
        external: true

## Configuring the default network

Instead of (or as well as) specifying your own networks, you can also change the settings of the app-wide default network by defining an entry under `networks` named `default`:

    version: 2

    services:
      web:
        build: .
        ports:
          - "8000:8000"
      db:
        image: postgres

    networks:
      default:
        # Use the overlay driver for multi-host communication
        driver: overlay

## Custom container network modes

The `docker` CLI command allows you to specify a custom network mode for a container with the `--net` option - for example, `--net=host` specifies that the container should use the same network namespace as the Docker host, and `--net=none` specifies that it should have no networking capabilities.

To make use of this in Compose, specify a `networks` list with a single item `host`, `bridge` or `none`:

    app:
      build: ./app
      networks: ["host"]

There is no equivalent to `--net=container:CONTAINER_NAME` in the v2 Compose file format. You should instead use networks to enable communication.
