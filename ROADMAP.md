# Roadmap

## Fig 1.1

- All this cool stuff: https://github.com/docker/fig/issues?q=milestone%3A1.1.0+

## Compose 1.2

- Project-wide rename and rebrand to Docker Compose, with new names for the command-line tool and configuration file
- Version specifier in configuration file
- A “fig watch” command which automatically kicks off builds while editing code
- It should be possible to somehow define hostnames for containers which work from the host machine, e.g. “mywebcontainer.local”. This is needed by e.g. apps comprising multiple web services which generate links to one another (e.g. a frontend website and a separate admin webapp)
- A way to share config between apps ([#318](https://github.com/docker/fig/issues/318))

## Future

- Fig uses Docker container names to separate and identify containers in different apps and belonging to different services within apps; this should really be done in a less hacky and more performant way, by attaching metadata to containers and doing the filtering on the server side. **This requires changes to the Docker daemon.**
- The config file should be parameterisable so that config can be partially modified for different environments (dev/test/staging/prod), passing in e.g. custom ports or volume mount paths. ([#426](https://github.com/docker/fig/issues/426))
- Fig’s brute-force “delete and recreate everything” approach is great for dev and testing, and with manual command-line scoping can be made to work in production (e.g. “fig up -d web” will update *just* the web service), but a smarter solution is needed, its logic probably based around convergence from “current” to “desired” app state.
- Compose should recommend a simple technique for zero-downtime deploys. This will likely involve new Docker networking methods that allow for a load balancer container to be dynamically hooked up to new app containers and disconnected from old ones.
