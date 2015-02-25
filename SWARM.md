Docker Compose/Swarm integration
================================

Eventually, Compose and Swarm aim to have full integration, meaning you can point a Compose app at a Swarm cluster and have it all just work as if you were using a single Docker host.

However, the current extent of integration is minimal: Compose can create containers on a Swarm cluster, but the majority of Compose apps won’t work out of the box unless all containers are scheduled on one host, defeating much of the purpose of using Swarm in the first place.

Still, Compose and Swarm can be useful in a “batch processing” scenario (where a large number of containers need to be spun up and down to do independent computation) or a “shared cluster” scenario (where multiple teams want to deploy apps on a cluster without worrying about where to put them).

A number of things need to happen before full integration is achieved, which are documented below.

Re-deploying containers with `docker-compose up`
------------------------------------------------

Repeated invocations of `docker-compose up` will not work reliably when used against a Swarm cluster because of an under-the-hood design problem; [this will be fixed](https://github.com/docker/fig/pull/972) in the next version of Compose. For now, containers must be completely removed and re-created:

    $ docker-compose kill
    $ docker-compose rm --force
    $ docker-compose up

Links and networking
--------------------

The primary thing stopping multi-container apps from working seamlessly on Swarm is getting them to talk to one another: enabling private communication between containers on different hosts hasn’t been solved in a non-hacky way.

Long-term, networking is [getting overhauled](https://github.com/docker/docker/issues/9983) in such a way that it’ll fit the multi-host model much better. For now, containers on different hosts cannot be linked. In the next version of Compose, linked services will be automatically scheduled on the same host; for now, this must be done manually (see “Co-scheduling containers” below).

`volumes_from` and `net: container`
-----------------------------------

For containers to share volumes or a network namespace, they must be scheduled on the same host - this is, after all, inherent to how both volumes and network namespaces work. In the next version of Compose, this co-scheduling will be automatic whenever `volumes_from` or `net: "container:..."` is specified; for now, containers which share volumes or a network namespace must be co-scheduled manually (see “Co-scheduling containers” below).

Co-scheduling containers
------------------------

For now, containers can be manually scheduled on the same host using Swarm’s [affinity filters](https://github.com/docker/swarm/blob/master/scheduler/filter/README.md#affinity-filter). Here’s a simple example:

```yaml
web:
  image: my-web-image
  links: ["db"]
  environment:
    - "affinity:container==myproject_db_*"
db:
  image: postgres
```

Here, we express an affinity filter on all web containers, saying that each one must run alongside a container whose name begins with `myproject_db_`.

- `myproject` is the common prefix Compose gives to all containers in your project, which is either generated from the name of the current directory or specified with `-p` or the `DOCKER_COMPOSE_PROJECT_NAME` environment variable.
- `*` is a wildcard, which works just like filename wildcards in a Unix shell.
