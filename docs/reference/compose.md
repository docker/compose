# docker compose

<!---MARKER_GEN_START-->
You can use the compose subcommand, `docker compose [-f <arg>...] [options] [COMMAND] [ARGS...]`, to build and manage
multiple services in Docker containers.

### Use `-f` to specify the name and path of one or more Compose files
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

Each configuration has a project name. Compose sets the project name using
the following mechanisms, in order of precedence:
- The `-p` command line flag
- The `COMPOSE_PROJECT_NAME` environment variable
- The top level `name:` variable from the config file (or the last `name:`
from a series of config files specified using `-f`)
- The `basename` of the project directory containing the config file (or
containing the first config file specified using `-f`)
- The `basename` of the current directory if no config file is specified
Project names must contain only lowercase letters, decimal digits, dashes,
and underscores, and must begin with a lowercase letter or decimal digit. If
the `basename` of the project directory or current directory violates this
constraint, you must use one of the other mechanisms.

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
Calling `docker compose --profile frontend up` starts the services with the profile `frontend` and services
without any specified profiles.
You can also enable multiple profiles, e.g. with `docker compose --profile frontend --profile debug up` the profiles `frontend` and `debug` is enabled.

Profiles can also be set by `COMPOSE_PROFILES` environment variable.

### Configuring parallelism

Use `--parallel` to specify the maximum level of parallelism for concurrent engine calls.
Calling `docker compose --parallel 1 pull` pulls the pullable images defined in the Compose file
one at a time. This can also be used to control build concurrency.

Parallelism can also be set by the `COMPOSE_PARALLEL_LIMIT` environment variable.

### Set up environment variables

You can set environment variables for various docker compose options, including the `-f`, `-p` and `--profiles` flags.

Setting the `COMPOSE_FILE` environment variable is equivalent to passing the `-f` flag,
`COMPOSE_PROJECT_NAME` environment variable does the same as the `-p` flag,
`COMPOSE_PROFILES` environment variable is equivalent to the `--profiles` flag
and `COMPOSE_PARALLEL_LIMIT` does the same as the `--parallel` flag.

If flags are explicitly set on the command line, the associated environment variable is ignored.

Setting the `COMPOSE_IGNORE_ORPHANS` environment variable to `true` stops docker compose from detecting orphaned
containers for the project.

Setting the `COMPOSE_MENU` environment variable to `false` disables the helper menu when running `docker compose up`
in attached mode. Alternatively, you can also run `docker compose up --menu=false` to disable the helper menu.

### Use Dry Run mode to test your command

Use `--dry-run` flag to test a command without changing your application stack state.
Dry Run mode shows you all the steps Compose applies when executing a command, for example:
```console
$ docker compose --dry-run up --build -d
[+] Pulling 1/1
 ✔ DRY-RUN MODE -  db Pulled                                                                                                                                                                                                               0.9s
[+] Running 10/8
 ✔ DRY-RUN MODE -    build service backend                                                                                                                                                                                                 0.0s
 ✔ DRY-RUN MODE -  ==> ==> writing image dryRun-754a08ddf8bcb1cf22f310f09206dd783d42f7dd                                                                                                                                                   0.0s
 ✔ DRY-RUN MODE -  ==> ==> naming to nginx-golang-mysql-backend                                                                                                                                                                            0.0s
 ✔ DRY-RUN MODE -  Network nginx-golang-mysql_default                                    Created                                                                                                                                           0.0s
 ✔ DRY-RUN MODE -  Container nginx-golang-mysql-db-1                                     Created                                                                                                                                           0.0s
 ✔ DRY-RUN MODE -  Container nginx-golang-mysql-backend-1                                Created                                                                                                                                           0.0s
 ✔ DRY-RUN MODE -  Container nginx-golang-mysql-proxy-1                                  Created                                                                                                                                           0.0s
 ✔ DRY-RUN MODE -  Container nginx-golang-mysql-db-1                                     Healthy                                                                                                                                           0.5s
 ✔ DRY-RUN MODE -  Container nginx-golang-mysql-backend-1                                Started                                                                                                                                           0.0s
 ✔ DRY-RUN MODE -  Container nginx-golang-mysql-proxy-1                                  Started                                     Started
```
From the example above, you can see that the first step is to pull the image defined by `db` service, then build the `backend` service.
Next, the containers are created. The `db` service is started, and the `backend` and `proxy` wait until the `db` service is healthy before starting.

Dry Run mode works with almost all commands. You cannot use Dry Run mode with a command that doesn't change the state of a Compose stack such as `ps`, `ls`, `logs` for example.

### Subcommands

| Name                            | Description                                                                             |
|:--------------------------------|:----------------------------------------------------------------------------------------|
| [`attach`](compose_attach.md)   | Attach local standard input, output, and error streams to a service's running container |
| [`build`](compose_build.md)     | Build or rebuild services                                                               |
| [`config`](compose_config.md)   | Parse, resolve and render compose file in canonical format                              |
| [`cp`](compose_cp.md)           | Copy files/folders between a service container and the local filesystem                 |
| [`create`](compose_create.md)   | Creates containers for a service                                                        |
| [`down`](compose_down.md)       | Stop and remove containers, networks                                                    |
| [`events`](compose_events.md)   | Receive real time events from containers                                                |
| [`exec`](compose_exec.md)       | Execute a command in a running container                                                |
| [`images`](compose_images.md)   | List images used by the created containers                                              |
| [`kill`](compose_kill.md)       | Force stop service containers                                                           |
| [`logs`](compose_logs.md)       | View output from containers                                                             |
| [`ls`](compose_ls.md)           | List running compose projects                                                           |
| [`pause`](compose_pause.md)     | Pause services                                                                          |
| [`port`](compose_port.md)       | Print the public port for a port binding                                                |
| [`ps`](compose_ps.md)           | List containers                                                                         |
| [`pull`](compose_pull.md)       | Pull service images                                                                     |
| [`push`](compose_push.md)       | Push service images                                                                     |
| [`restart`](compose_restart.md) | Restart service containers                                                              |
| [`rm`](compose_rm.md)           | Removes stopped service containers                                                      |
| [`run`](compose_run.md)         | Run a one-off command on a service                                                      |
| [`scale`](compose_scale.md)     | Scale services                                                                          |
| [`start`](compose_start.md)     | Start services                                                                          |
| [`stats`](compose_stats.md)     | Display a live stream of container(s) resource usage statistics                         |
| [`stop`](compose_stop.md)       | Stop services                                                                           |
| [`top`](compose_top.md)         | Display the running processes                                                           |
| [`unpause`](compose_unpause.md) | Unpause services                                                                        |
| [`up`](compose_up.md)           | Create and start containers                                                             |
| [`version`](compose_version.md) | Show the Docker Compose version information                                             |
| [`wait`](compose_wait.md)       | Block until the first service container stops                                           |
| [`watch`](compose_watch.md)     | Watch build context for service and rebuild/refresh containers when files are updated   |


### Options

| Name                   | Type          | Default | Description                                                                                         |
|:-----------------------|:--------------|:--------|:----------------------------------------------------------------------------------------------------|
| `--all-resources`      | `bool`        |         | Include all resources, even those not used by services                                              |
| `--ansi`               | `string`      | `auto`  | Control when to print ANSI control characters ("never"\|"always"\|"auto")                           |
| `--compatibility`      | `bool`        |         | Run compose in backward compatibility mode                                                          |
| `--dry-run`            | `bool`        |         | Execute command in dry run mode                                                                     |
| `--env-file`           | `stringArray` |         | Specify an alternate environment file                                                               |
| `-f`, `--file`         | `stringArray` |         | Compose configuration files                                                                         |
| `--parallel`           | `int`         | `-1`    | Control max parallelism, -1 for unlimited                                                           |
| `--profile`            | `stringArray` |         | Specify a profile to enable                                                                         |
| `--progress`           | `string`      | `auto`  | Set type of progress output (auto, tty, plain, json, quiet)                                         |
| `--project-directory`  | `string`      |         | Specify an alternate working directory<br>(default: the path of the, first specified, Compose file) |
| `-p`, `--project-name` | `string`      |         | Project name                                                                                        |


<!---MARKER_GEN_END-->

## Description

You can use the compose subcommand, `docker compose [-f <arg>...] [options] [COMMAND] [ARGS...]`, to build and manage
multiple services in Docker containers.

### Use `-f` to specify the name and path of one or more Compose files
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

Each configuration has a project name. Compose sets the project name using
the following mechanisms, in order of precedence:
- The `-p` command line flag
- The `COMPOSE_PROJECT_NAME` environment variable
- The top level `name:` variable from the config file (or the last `name:`
from a series of config files specified using `-f`)
- The `basename` of the project directory containing the config file (or
containing the first config file specified using `-f`)
- The `basename` of the current directory if no config file is specified
Project names must contain only lowercase letters, decimal digits, dashes,
and underscores, and must begin with a lowercase letter or decimal digit. If
the `basename` of the project directory or current directory violates this
constraint, you must use one of the other mechanisms.

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
Calling `docker compose --profile frontend up` starts the services with the profile `frontend` and services
without any specified profiles.
You can also enable multiple profiles, e.g. with `docker compose --profile frontend --profile debug up` the profiles `frontend` and `debug` is enabled.

Profiles can also be set by `COMPOSE_PROFILES` environment variable.

### Configuring parallelism

Use `--parallel` to specify the maximum level of parallelism for concurrent engine calls.
Calling `docker compose --parallel 1 pull` pulls the pullable images defined in the Compose file
one at a time. This can also be used to control build concurrency.

Parallelism can also be set by the `COMPOSE_PARALLEL_LIMIT` environment variable.

### Set up environment variables

You can set environment variables for various docker compose options, including the `-f`, `-p` and `--profiles` flags.

Setting the `COMPOSE_FILE` environment variable is equivalent to passing the `-f` flag,
`COMPOSE_PROJECT_NAME` environment variable does the same as the `-p` flag,
`COMPOSE_PROFILES` environment variable is equivalent to the `--profiles` flag
and `COMPOSE_PARALLEL_LIMIT` does the same as the `--parallel` flag.

If flags are explicitly set on the command line, the associated environment variable is ignored.

Setting the `COMPOSE_IGNORE_ORPHANS` environment variable to `true` stops docker compose from detecting orphaned
containers for the project.

Setting the `COMPOSE_MENU` environment variable to `false` disables the helper menu when running `docker compose up`
in attached mode. Alternatively, you can also run `docker compose up --menu=false` to disable the helper menu.

### Use Dry Run mode to test your command

Use `--dry-run` flag to test a command without changing your application stack state.
Dry Run mode shows you all the steps Compose applies when executing a command, for example:
```console
$ docker compose --dry-run up --build -d
[+] Pulling 1/1
 ✔ DRY-RUN MODE -  db Pulled                                                                                                                                                                                                               0.9s
[+] Running 10/8
 ✔ DRY-RUN MODE -    build service backend                                                                                                                                                                                                 0.0s
 ✔ DRY-RUN MODE -  ==> ==> writing image dryRun-754a08ddf8bcb1cf22f310f09206dd783d42f7dd                                                                                                                                                   0.0s
 ✔ DRY-RUN MODE -  ==> ==> naming to nginx-golang-mysql-backend                                                                                                                                                                            0.0s
 ✔ DRY-RUN MODE -  Network nginx-golang-mysql_default                                    Created                                                                                                                                           0.0s
 ✔ DRY-RUN MODE -  Container nginx-golang-mysql-db-1                                     Created                                                                                                                                           0.0s
 ✔ DRY-RUN MODE -  Container nginx-golang-mysql-backend-1                                Created                                                                                                                                           0.0s
 ✔ DRY-RUN MODE -  Container nginx-golang-mysql-proxy-1                                  Created                                                                                                                                           0.0s
 ✔ DRY-RUN MODE -  Container nginx-golang-mysql-db-1                                     Healthy                                                                                                                                           0.5s
 ✔ DRY-RUN MODE -  Container nginx-golang-mysql-backend-1                                Started                                                                                                                                           0.0s
 ✔ DRY-RUN MODE -  Container nginx-golang-mysql-proxy-1                                  Started                                     Started
```
From the example above, you can see that the first step is to pull the image defined by `db` service, then build the `backend` service.  
Next, the containers are created. The `db` service is started, and the `backend` and `proxy` wait until the `db` service is healthy before starting.

Dry Run mode works with almost all commands. You cannot use Dry Run mode with a command that doesn't change the state of a Compose stack such as `ps`, `ls`, `logs` for example.  
