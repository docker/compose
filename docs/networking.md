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

> **Note:** Compose's networking support is experimental, and must be explicitly enabled with the `docker-compose --x-networking` flag.

Compose sets up a single default [network](/engine/reference/commandline/network_create.md) for your app. Each container for a service joins the default network and is both *reachable* by other containers on that network, and *discoverable* by them at a hostname identical to the service's name.

> **Note:** Your app's network is given the same name as the "project name", which is based on the name of the directory it lives in. See the [Command line overview](reference/docker-compose.md) for how to override it.

For example, suppose your app is in a directory called `myapp`, and your `docker-compose.yml` looks like this:

    web:
      build: .
      ports:
        - "8000:8000"
    db:
      image: postgres

When you run `docker-compose --x-networking up`, the following happens:

1. A network called `myapp` is created.
2. A container is created using `web`'s configuration. It joins the network `myapp` under the name `web`.
3. A container is created using `db`'s configuration. It joins the network `myapp` under the name `db`.

Each container can now look up the hostname `web` or `db` and get back the appropriate container's IP address. For example, `web`'s application code could connect to the URL `postgres://db:5432` and start using the Postgres database.

Because `web` explicitly maps a port, it's also accessible from the outside world via port 8000 on your Docker host's network interface.

## Updating containers

If you make a configuration change to a service and run `docker-compose up` to update it, the old container will be removed and the new one will join the network under a different IP address but the same name. Running containers will be able to look up that name and connect to the new address, but the old address will stop working.

If any containers have connections open to the old container, they will be closed. It is a container's responsibility to detect this condition, look up the name again and reconnect.

## Configure how services are published

By default, containers for each service are published on the network with the same name as the service. If you want to change the name, or stop containers from being discoverable at all, you can use the `hostname` option:

    web:
      build: .
      hostname: "my-web-application"

This will also change the hostname inside the container, so that the `hostname` command will return `my-web-application`.

## Scaling services

If you create multiple containers for a service with `docker-compose scale`, each container will join the network with the same name. For example, if you run `docker-compose scale web=3`, then 3 containers will join the network under the name `web`. Inside any container on the network, looking up the name `web` will return the IP address of one of them, but Docker and Compose do not provide any guarantees about which one.

This limitation will be addressed in a future version of Compose, where a load balancer will join under the service name and balance traffic between the service's containers in a configurable manner.

## Links

Docker links are a one-way, single-host communication system. They should now be considered deprecated, and you should update your app to use networking instead. In the majority of cases, this will simply involve removing the `links` sections from your `docker-compose.yml`.

## Specifying the network driver

By default, Compose uses the `bridge` driver when creating the app’s network. The Docker Engine provides one other driver out-of-the-box: `overlay`, which implements secure communication between containers on different hosts (see the next section for how to set up and use the `overlay` driver). Docker also allows you to install [custom network drivers](/engine/extend/plugins_network.md).

You can specify which one to use with the `--x-network-driver` flag:

    $ docker-compose --x-networking --x-network-driver=overlay up

## Multi-host networking

(TODO: talk about Swarm and the overlay driver)

## Custom container network modes

Compose allows you to specify a custom network mode for a service with the `net` option - for example, `net: "host"` specifies that its containers should use the same network namespace as the Docker host, and `net: "none"` specifies that they should have no networking capabilities.

If a service specifies the `net` option, its containers will *not* join the app’s network and will not be able to communicate with other services in the app.

If *all* services in an app specify the `net` option, a network will not be created at all.
