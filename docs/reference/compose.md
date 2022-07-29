# docker compose

<!---MARKER_GEN_START-->
Docker Compose

### Subcommands

| Name | Description |
| --- | --- |
| [`build`](compose_build.md) | Build or rebuild services |
| [`convert`](compose_convert.md) | Converts the compose file to platform's canonical format |
| [`cp`](compose_cp.md) | Copy files/folders between a service container and the local filesystem |
| [`create`](compose_create.md) | Creates containers for a service. |
| [`down`](compose_down.md) | Stop and remove containers, networks |
| [`events`](compose_events.md) | Receive real time events from containers. |
| [`exec`](compose_exec.md) | Execute a command in a running container. |
| [`images`](compose_images.md) | List images used by the created containers |
| [`kill`](compose_kill.md) | Force stop service containers. |
| [`logs`](compose_logs.md) | View output from containers |
| [`ls`](compose_ls.md) | List running compose projects |
| [`pause`](compose_pause.md) | Pause services |
| [`port`](compose_port.md) | Print the public port for a port binding. |
| [`ps`](compose_ps.md) | List containers |
| [`pull`](compose_pull.md) | Pull service images |
| [`push`](compose_push.md) | Push service images |
| [`restart`](compose_restart.md) | Restart service containers |
| [`rm`](compose_rm.md) | Removes stopped service containers |
| [`run`](compose_run.md) | Run a one-off command on a service. |
| [`start`](compose_start.md) | Start services |
| [`stop`](compose_stop.md) | Stop services |
| [`top`](compose_top.md) | Display the running processes |
| [`unpause`](compose_unpause.md) | Unpause services |
| [`up`](compose_up.md) | Create and start containers |
| [`version`](compose_version.md) | Show the Docker Compose version information |


### Options

| Name | Type | Default | Description |
| --- | --- | --- | --- |
| `--ansi` | `string` | `auto` | Control when to print ANSI control characters ("never"\|"always"\|"auto") |
| `--compatibility` |  |  | Run compose in backward compatibility mode |
| `--env-file` | `string` |  | Specify an alternate environment file. |
| `-f`, `--file` | `stringArray` |  | Compose configuration files |
| `--profile` | `stringArray` |  | Specify a profile to enable |
| `--project-directory` | `string` |  | Specify an alternate working directory
(default: the path of the, first specified, Compose file) |
| `-p`, `--project-name` | `string` |  | Project name |


<!---MARKER_GEN_END-->

## Description

You can use compose subcommand, `docker compose [-f <arg>...] [options] [COMMAND] [ARGS...]`, to build and manage
multiple services in Docker containers.

### Use `-f` to specify name and path of one or more Compose files
Use the `-f` flag to specify the location of a Compose configuration file.

#### Specifying multiple Compose files
You can supply multiple `-f` configuration files. When you supply multiple files, Compose combines them into a single
configuration. Compose builds the configuration in the order you supply the files. Subsequent files override and add
to their predecessors.

For example, consider this command line:

```console
$ docker compose -f docker-compose.yml -f docker-compose.admin.yml run backup_db
```

The `docker-compose.yml` file might specify a `webapp` service.

```yaml
services:
  webapp:
    image: examples/web
    ports:
      - "8000:8000"
    volumes:
      - "/data"
```
If the `docker-compose.admin.yml` also specifies this same service, any matching fields override the previous file.
New values, add to the `webapp` service configuration.

```yaml
services:
  webapp:
    build: .
    environment:
      - DEBUG=1
```

When you use multiple Compose files, all paths in the files are relative to the first configuration file specified
with `-f`. You can use the `--project-directory` option to override this base path.

Use a `-f` with `-` (dash) as the filename to read the configuration from stdin. When stdin is used all paths in the
configuration are relative to the current working directory.

The `-f` flag is optional. If you don’t provide this flag on the command line, Compose traverses the working directory
and its parent directories looking for a `compose.yaml` or `docker-compose.yaml` file.

#### Specifying a path to a single Compose file
You can use the `-f` flag to specify a path to a Compose file that is not located in the current directory, either
from the command line or by setting up a `COMPOSE_FILE` environment variable in your shell or in an environment file.

For an example of using the `-f` option at the command line, suppose you are running the Compose Rails sample, and
have a `compose.yaml` file in a directory called `sandbox/rails`. You can use a command like `docker compose pull` to
get the postgres image for the db service from anywhere by using the `-f` flag as follows:

```console
$ docker compose -f ~/sandbox/rails/compose.yaml pull db
```

### Use `-p` to specify a project name

Each configuration has a project name. If you supply a `-p` flag, you can specify a project name. If you don’t
specify the flag, Compose uses the current directory name.
Project name can also be set by `COMPOSE_PROJECT_NAME` environment variable.

Most compose subcommand can be ran without a compose file, just passing
project name to retrieve the relevant resources.

```console
$ docker compose -p my_project ps -a
NAME                 SERVICE    STATUS     PORTS
my_project_demo_1    demo       running

$ docker compose -p my_project logs
demo_1  | PING localhost (127.0.0.1): 56 data bytes
demo_1  | 64 bytes from 127.0.0.1: seq=0 ttl=64 time=0.095 ms
```

### Use profiles to enable optional services

Use `--profile` to specify one or more active profiles
Calling `docker compose --profile frontend up` will start the services with the profile `frontend` and services
without any specified profiles.
You can also enable multiple profiles, e.g. with `docker compose --profile frontend --profile debug up` the profiles `frontend` and `debug` will be enabled.

Profiles can also be set by `COMPOSE_PROFILES` environment variable.

### Set up environment variables

You can set environment variables for various docker compose options, including the `-f`, `-p` and `--profiles` flags.

Setting the `COMPOSE_FILE` environment variable is equivalent to passing the `-f` flag,
`COMPOSE_PROJECT_NAME` environment variable does the same for to the `-p` flag,
and so does `COMPOSE_PROFILES` environment variable for to the `--profiles` flag.

If flags are explicitly set on command line, associated environment variable is ignored

Setting the `COMPOSE_IGNORE_ORPHANS` environment variable to `true` will stop docker compose from detecting orphaned
containers for the project.
