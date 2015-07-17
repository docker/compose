<!--[metadata]>
+++
title = "up"
description = "Builds, (re)creates, starts, and attaches to containers for a service."
keywords = ["fig, composition, compose, docker, orchestration, cli,  up"]
[menu.main]
parent = "smn_compose_cli"
+++
<![end-metadata]-->

# up

```
Usage: up [options] [SERVICE...]

Options:
--allow-insecure-ssl   Allow insecure connections to the docker
                       registry
-d                     Detached mode: Run containers in the background,
                       print new container names.
--no-color             Produce monochrome output.
--no-deps              Don't start linked services.
--x-smart-recreate     Only recreate containers whose configuration or
                       image needs to be updated. (EXPERIMENTAL)
--no-recreate          If containers already exist, don't recreate them.
--no-build             Don't build an image, even if it's missing
-t, --timeout TIMEOUT  Use this timeout in seconds for container shutdown
                       when attached or when containers are already
                       running. (default: 10)
```

Builds, (re)creates, starts, and attaches to containers for a service.

Linked services will be started, unless they are already running.

By default, `docker-compose up` will aggregate the output of each container and,
when it exits, all containers will be stopped. Running `docker-compose up -d`,
will start the containers in the background and leave them running.

By default, if there are existing containers for a service, `docker-compose up` will stop and recreate them (preserving mounted volumes with [volumes-from]), so that changes in `docker-compose.yml` are picked up. If you do not want containers stopped and recreated, use `docker-compose up --no-recreate`. This will still start any stopped containers, if needed.

[volumes-from]: http://docs.docker.io/en/latest/use/working_with_volumes/
