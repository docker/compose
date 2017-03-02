# Roadmap

## An even better tool for development environments

Compose is a great tool for development environments, but it could be even better. For example:

- It should be possible to define hostnames for containers which work from the host machine, e.g. “mywebcontainer.local”. This is needed by apps comprising multiple web services which generate links to one another (e.g. a frontend website and a separate admin webapp)

## More than just development environments

Compose currently works really well in development, but we want to make the Compose file format better for test, staging, and production environments. To support these use cases, there will need to be improvements to the file format, improvements to the command-line tool, integrations with other tools, and perhaps new tools altogether.

Some specific things we are considering:

- Compose currently will attempt to get your application into the correct state when running `up`, but it has a number of shortcomings:
  - It should roll back to a known good state if it fails.
  - It should allow a user to check the actions it is about to perform before running them.
- It should be possible to partially modify the config file for different environments (dev/test/staging/prod), passing in e.g. custom ports, volume mount paths, or volume drivers. ([#1377](https://github.com/docker/compose/issues/1377))
- Compose should recommend a technique for zero-downtime deploys. ([#1786](https://github.com/docker/compose/issues/1786))
- It should be possible to continuously attempt to keep an application in the correct state, instead of just performing `up` a single time.

## Integration with Swarm

Compose should integrate really well with Swarm so you can take an application you've developed on your laptop and run it on a Swarm cluster.

The current state of integration is documented in [SWARM.md](SWARM.md).

## Applications spanning multiple teams

Compose works well for applications that are in a single repository and depend on services that are hosted on Docker Hub. If your application depends on another application within your organisation, Compose doesn't work as well.

There are several ideas about how this could work, such as [including external files](https://github.com/docker/fig/issues/318).
