<!--[metadata]>
+++
title = "docker-compose"
description = "docker-compose Command Binary"
keywords = ["fig, composition, compose, docker, orchestration, cli,  docker-compose"]
[menu.main]
parent = "smn_compose_cli"
weight=-2
+++
<![end-metadata]-->


# docker-compose Command

```
Usage:
  docker-compose [-f=<arg>...] [options] [COMMAND] [ARGS...]
  docker-compose -h|--help

Options:
  -f, --file FILE           Specify an alternate compose file (default: docker-compose.yml)
  -p, --project-name NAME   Specify an alternate project name (default: directory name)
  --verbose                 Show more output
  -v, --version             Print version and exit

Commands:
  build              Build or rebuild services
  help               Get help on a command
  kill               Kill containers
  logs               View output from containers
  pause              Pause services
  port               Print the public port for a port binding
  ps                 List containers
  pull               Pulls service images
  restart            Restart services
  rm                 Remove stopped containers
  run                Run a one-off command
  scale              Set number of containers for a service
  start              Start services
  stop               Stop services
  unpause            Unpause services
  up                 Create and start containers
  clean              Remove orphan containers
  migrate-to-labels  Recreate containers to add labels
  version            Show the Docker-Compose version information
```

The Docker Compose binary. You use this command to build and manage multiple
services in Docker containers.

Use the `-f` flag to specify the location of a Compose configuration file. You
can supply multiple `-f` configuration files. When you supply multiple files,
Compose combines them into a single configuration. Compose builds the
configuration in the order you supply the files. Subsequent files override and
add to their successors.

For example, consider this command line:

```
$ docker-compose -f docker-compose.yml -f docker-compose.admin.yml run backup_db`
```

The `docker-compose.yml` file might specify a `webapp` service.

```
webapp:
  image: examples/web
  ports:
    - "8000:8000"
  volumes:
    - "/data"
```

If the `docker-compose.admin.yml` also specifies this same service, any matching
fields will override the previous file. New values, add to the `webapp` service
configuration.

```
webapp:
  build: .
  environment:
    - DEBUG=1
```

Use a `-f` with `-` (dash) as the filename to read the configuration from
stdin. When stdin is used all paths in the configuration are
relative to the current working directory.

The `-f` flag is optional. If you don't provide this flag on the command line,
Compose traverses the working directory and its subdirectories looking for a
`docker-compose.yml` and a `docker-compose.override.yml` file. You must
supply at least the `docker-compose.yml` file. If both files are present,
Compose combines the two files into a single configuration. The configuration
in the `docker-compose.override.yml` file is applied over and in addition to
the values in the `docker-compose.yml` file.

See also the `COMPOSE_FILE` [environment variable](overview.md#compose-file).

Each configuration has a project name. If you supply a `-p` flag, you can
specify a project name. If you don't specify the flag, Compose uses the current
directory name. See also the `COMPOSE_PROJECT_NAME` [environment variable](
overview.md#compose-project-name)


## Where to go next

* [CLI environment variables](overview.md)
* [Command line reference](index.md)
