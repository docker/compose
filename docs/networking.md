<!--[metadata]>
+++
title = "Networking in Compose"
description = "How Compose sets up networking between containers"
keywords = ["documentation, docs,  docker, compose, orchestration, containers, networking"]
[menu.main]
parent="workw_compose"
weight=21
+++
<![end-metadata]-->


# Networking in Compose

> **Note:** This document only applies if you're using [version 2 of the Compose file format](compose-file.md#versioning). Networking features are not supported for version 1 (legacy) Compose files.

By default Compose sets up a single
[network](https://docs.docker.com/engine/reference/commandline/network_create/) for your app. Each
container for a service joins the default network and is both *reachable* by
other containers on that network, and *discoverable* by them at a hostname
identical to the container name.

> **Note:** Your app's network is given a name based on the "project name",
> which is based on the name of the directory it lives in. You can override the
> project name with either the [`--project-name`
> flag](reference/overview.md) or the [`COMPOSE_PROJECT_NAME` environment
> variable](reference/envvars.md#compose-project-name).

For example, suppose your app is in a directory called `myapp`, and your `docker-compose.yml` looks like this:

    version: '2'

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

Links allow you to define extra aliases by which a service is reachable from another service. They are not required to enable services to communicate - by default, any service can reach any other service at that service's name. In the following example, `db` is reachable from `web` at the hostnames `db` and `database`:

    version: '2'
    services:
      web:
        build: .
        links:
          - "db:database"
      db:
        image: postgres

See the [links reference](compose-file.md#links) for more information.

## Multi-host networking

When [deploying a Compose application to a Swarm cluster](swarm.md), you can make use of the built-in `overlay` driver to enable multi-host communication between containers with no changes to your Compose file or application code.

Consult the [Getting started with multi-host networking](https://docs.docker.com/engine/userguide/networking/get-started-overlay/) to see how to set up a Swarm cluster. The cluster will use the `overlay` driver by default, but you can specify it explicitly if you prefer - see below for how to do this.

## Specifying custom networks

Instead of just using the default app network, you can specify your own networks with the top-level `networks` key. This lets you create more complex topologies and specify [custom network drivers](https://docs.docker.com/engine/extend/plugins_network/) and options. You can also use it to connect services to externally-created networks which aren't managed by Compose.

Each service can specify what networks to connect to with the *service-level* `networks` key, which is a list of names referencing entries under the *top-level* `networks` key.

Here's an example Compose file defining two custom networks. The `proxy` service is isolated from the `db` service, because they do not share a network in common - only `app` can talk to both.

    version: '2'

    services:
      proxy:
        build: ./proxy
        networks:
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
        # Use a custom driver
        driver: custom-driver-1
      back:
        # Use a custom driver which takes special options
        driver: custom-driver-2
        driver_opts:
          foo: "1"
          bar: "2"

Networks can be configured with static IP addresses by setting the [ipv4_address and/or ipv6_address](compose-file.md#ipv4-address-ipv6-address) for each attached network.

For full details of the network configuration options available, see the following references:

- [Top-level `networks` key](compose-file.md#network-configuration-reference)
- [Service-level `networks` key](compose-file.md#networks)

## Configuring the default network

Instead of (or as well as) specifying your own networks, you can also change the settings of the app-wide default network by defining an entry under `networks` named `default`:

    version: '2'

    services:
      web:
        build: .
        ports:
          - "8000:8000"
      db:
        image: postgres

    networks:
      default:
        # Use a custom driver
        driver: custom-driver-1

## Using a pre-existing network

If you want your containers to join a pre-existing network, use the [`external` option](compose-file.md#network-configuration-reference):

    networks:
      default:
        external:
          name: my-pre-existing-network

Instead of attemping to create a network called `[projectname]_default`, Compose will look for a network called `my-pre-existing-network` and connect your app's containers to it.
