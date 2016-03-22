<!--[metadata]>
+++
title = "Using Compose with Swarm"
description = "How to use Compose and Swarm together to deploy apps to multi-host clusters"
keywords = ["documentation, docs,  docker, compose, orchestration, containers, swarm"]
[menu.main]
parent="workw_compose"
+++
<![end-metadata]-->


# Using Compose with Swarm

Docker Compose and [Docker Swarm](/swarm/overview) aim to have full integration, meaning
you can point a Compose app at a Swarm cluster and have it all just work as if
you were using a single Docker host.

The actual extent of integration depends on which version of the [Compose file
format](compose-file.md#versioning) you are using:

1.  If you're using version 1 along with `links`, your app will work, but Swarm
    will schedule all containers on one host, because links between containers
    do not work across hosts with the old networking system.

2. If you're using version 2, your app should work with no changes:

    - subject to the [limitations](#limitations) described below,

    - as long as the Swarm cluster is configured to use the [overlay driver](https://docs.docker.com/engine/userguide/networking/dockernetworks/#an-overlay-network),
      or a custom driver which supports multi-host networking.

Read [Get started with multi-host networking](https://docs.docker.com/engine/userguide/networking/get-started-overlay/) to see how to
set up a Swarm cluster with [Docker Machine](/machine/overview) and the overlay driver. Once you've got it running, deploying your app to it should be as simple as:

    $ eval "$(docker-machine env --swarm <name of swarm master machine>)"
    $ docker-compose up


## Limitations

### Building images

Swarm can build an image from a Dockerfile just like a single-host Docker
instance can, but the resulting image will only live on a single node and won't
be distributed to other nodes.

If you want to use Compose to scale the service in question to multiple nodes,
you'll have to build it yourself, push it to a registry (e.g. the Docker Hub)
and reference it from `docker-compose.yml`:

    $ docker build -t myusername/web .
    $ docker push myusername/web

    $ cat docker-compose.yml
    web:
      image: myusername/web

    $ docker-compose up -d
    $ docker-compose scale web=3

### Multiple dependencies

If a service has multiple dependencies of the type which force co-scheduling
(see [Automatic scheduling](#automatic-scheduling) below), it's possible that
Swarm will schedule the dependencies on different nodes, making the dependent
service impossible to schedule. For example, here `foo` needs to be co-scheduled
with `bar` and `baz`:

    version: "2"
    services:
      foo:
        image: foo
        volumes_from: ["bar"]
        network_mode: "service:baz"
      bar:
        image: bar
      baz:
        image: baz

The problem is that Swarm might first schedule `bar` and `baz` on different
nodes (since they're not dependent on one another), making it impossible to
pick an appropriate node for `foo`.

To work around this, use [manual scheduling](#manual-scheduling) to ensure that
all three services end up on the same node:

    version: "2"
    services:
      foo:
        image: foo
        volumes_from: ["bar"]
        network_mode: "service:baz"
        environment:
          - "constraint:node==node-1"
      bar:
        image: bar
        environment:
          - "constraint:node==node-1"
      baz:
        image: baz
        environment:
          - "constraint:node==node-1"

### Host ports and recreating containers

If a service maps a port from the host, e.g. `80:8000`, then you may get an
error like this when running `docker-compose up` on it after the first time:

    docker: Error response from daemon: unable to find a node that satisfies
    container==6ab2dfe36615ae786ef3fc35d641a260e3ea9663d6e69c5b70ce0ca6cb373c02.

The usual cause of this error is that the container has a volume (defined either
in its image or in the Compose file) without an explicit mapping, and so in
order to preserve its data, Compose has directed Swarm to schedule the new
container on the same node as the old container. This results in a port clash.

There are two viable workarounds for this problem:

-   Specify a named volume, and use a volume driver which is capable of mounting
    the volume into the container regardless of what node it's scheduled on.

    Compose does not give Swarm any specific scheduling instructions if a
    service uses only named volumes.

        version: "2"

        services:
          web:
            build: .
            ports:
              - "80:8000"
            volumes:
              - web-logs:/var/log/web

        volumes:
          web-logs:
            driver: custom-volume-driver

-   Remove the old container before creating the new one. You will lose any data
    in the volume.

        $ docker-compose stop web
        $ docker-compose rm -f web
        $ docker-compose up web


## Scheduling containers

### Automatic scheduling

Some configuration options will result in containers being automatically
scheduled on the same Swarm node to ensure that they work correctly. These are:

-   `network_mode: "service:..."` and `network_mode: "container:..."` (and
    `net: "container:..."` in the version 1 file format).

-   `volumes_from`

-   `links`

### Manual scheduling

Swarm offers a rich set of scheduling and affinity hints, enabling you to
control where containers are located. They are specified via container
environment variables, so you can use Compose's `environment` option to set
them.

    # Schedule containers on a specific node
    environment:
      - "constraint:node==node-1"

    # Schedule containers on a node that has the 'storage' label set to 'ssd'
    environment:
      - "constraint:storage==ssd"

    # Schedule containers where the 'redis' image is already pulled
    environment:
      - "affinity:image==redis"

For the full set of available filters and expressions, see the [Swarm
documentation](/swarm/scheduler/filter.md).
