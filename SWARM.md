Docker Compose/Swarm integration
================================

Eventually, Compose and Swarm aim to have full integration, meaning you can point a Compose app at a Swarm cluster and have it all just work as if you were using a single Docker host.

However, the current extent of integration is minimal: Compose can create containers on a Swarm cluster, but the majority of Compose apps won’t work out of the box unless all containers are scheduled on one host, defeating much of the purpose of using Swarm in the first place.

Still, Compose and Swarm can be useful in a “batch processing” scenario (where a large number of containers need to be spun up and down to do independent computation) or a “shared cluster” scenario (where multiple teams want to deploy apps on a cluster without worrying about where to put them).

A number of things need to happen before full integration is achieved, which are documented below.

Links and networking
--------------------

The primary thing stopping multi-container apps from working seamlessly on Swarm is getting them to talk to one another: enabling private communication between containers on different hosts hasn’t been solved in a non-hacky way.

Long-term, networking is [getting overhauled](https://github.com/docker/docker/issues/9983) in such a way that it’ll fit the multi-host model much better. For now, **linked containers are automatically scheduled on the same host**.

Building
--------

`docker build` against a Swarm cluster is not implemented, so for now the `build` option will not work - you will need to manually build your service's image, push it somewhere and use `image` to instruct Compose to pull it. Here's an example using the Docker Hub:

    $ docker build -t myusername/web .
    $ docker push myusername/web
    $ cat docker-compose.yml
    web:
      image: myusername/web
      links: ["db"]
    db:
      image: postgres
    $ docker-compose up -d
