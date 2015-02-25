# Roadmap

## More than just development environments

Over time we will extend Compose's remit to cover test, staging and production environments. This is not a simple task, and will take many incremental improvements such as:

- Compose’s brute-force “delete and recreate everything” approach is great for dev and testing, but it not sufficient for production environments. You should be able to define a "desired" state that Compose will intelligently converge to.
- It should be possible to partially modify the config file for different environments (dev/test/staging/prod), passing in e.g. custom ports or volume mount paths. ([#426](https://github.com/docker/fig/issues/426))
- Compose should recommend a technique for zero-downtime deploys.

## Integration with Swarm

Compose should integrate really well with Swarm so you can take an application you've developed on your laptop and run it on a Swarm cluster.

The current state of integration is documented in [SWARM.md](SWARM.md).

## Applications spanning multiple teams

Compose works well for applications that are in a single repository and depend on services that are hosted on Docker Hub. If your application depends on another application within your organisation, Compose doesn't work as well.

There are several ideas about how this could work, such as [including external files](https://github.com/docker/fig/issues/318).

## An even better tool for development environments

Compose is a great tool for development environments, but it could be even better. For example:

- [Compose could watch your code and automatically kick off builds when something changes.](https://github.com/docker/fig/issues/184)
- It should be possible to define hostnames for containers which work from the host machine, e.g. “mywebcontainer.local”. This is needed by apps comprising multiple web services which generate links to one another (e.g. a frontend website and a separate admin webapp)
