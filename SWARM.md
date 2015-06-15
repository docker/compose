Docker Compose/Swarm integration
================================

Eventually, Compose and Swarm aim to have full integration, meaning you can point a Compose app at a Swarm cluster and have it all just work as if you were using a single Docker host.

However, integration is currently incomplete: Compose can create containers on a Swarm cluster, but the majority of Compose apps won’t work out of the box unless all containers are scheduled on one host, because links between containers do not work across hosts.

Docker networking is [getting overhauled](https://github.com/docker/libnetwork) in such a way that it’ll fit the multi-host model much better. For now, linked containers are automatically scheduled on the same host.

Building
--------

Swarm can build an image from a Dockerfile just like a single-host Docker instance can, but the resulting image will only live on a single node and won't be distributed to other nodes.

If you want to use Compose to scale the service in question to multiple nodes, you'll have to build it yourself, push it to a registry (e.g. the Docker Hub) and reference it from `docker-compose.yml`:

    $ docker build -t myusername/web .
    $ docker push myusername/web

    $ cat docker-compose.yml
    web:
      image: myusername/web

    $ docker-compose up -d
    $ docker-compose scale web=3

Scheduling
----------

Swarm offers a rich set of scheduling and affinity hints, enabling you to control where containers are located. They are specified via container environment variables, so you can use Compose's `environment` option to set them.

    environment:
      # Schedule containers on a node that has the 'storage' label set to 'ssd'
      - "constraint:storage==ssd"

      # Schedule containers where the 'redis' image is already pulled
      - "affinity:image==redis"

For the full set of available filters and expressions, see the [Swarm documentation](https://docs.docker.com/swarm/scheduler/filter/).
