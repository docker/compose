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
  docker-compose [options] [COMMAND] [ARGS...]
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
  migrate-to-labels  Recreate containers to add labels
```

The Docker Compose binary. You use this command to build and manage multiple services in Docker containers.

Use the `-f` flag to specify the location of a Compose configuration file. This
flag is optional. If you don't provide this flag. Compose looks for a file named
`docker-compose.yml` in the  working directory. If the file is not found,
Compose looks in each parent directory successively, until it finds the file.

Use a `-` as the filename to read configuration file from stdin. When stdin is
used all paths in the configuration are relative to the current working
directory.

Each configuration can has a project name. If you supply a `-p` flag, you can specify a project name. If you don't specify the flag, Compose uses the current directory name.
