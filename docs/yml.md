<!--[metadata]>
+++
title = "docker-compose.yml reference"
description = "docker-compose.yml reference"
keywords = ["fig, composition, compose,  docker"]
[menu.main]
parent="smn_compose_ref"
+++
<![end-metadata]-->


# docker-compose.yml reference

Each service defined in `docker-compose.yml` must specify exactly one of
`image` or `build`. Other keys are optional, and are analogous to their
`docker run` command-line counterparts.

As with `docker run`, options specified in the Dockerfile (e.g., `CMD`,
`EXPOSE`, `VOLUME`, `ENV`) are respected by default - you don't need to
specify them again in `docker-compose.yml`.

### build

Path to a directory containing a Dockerfile. When the value supplied is a
relative path, it is interpreted as relative to the location of the yml file
itself. This directory is also the build context that is sent to the Docker daemon.

Compose will build and tag it with a generated name, and use that image thereafter.

    build: /path/to/build/dir

Using `build` together with `image` is not allowed. Attempting to do so results in an error.

### cap_add, cap_drop

Add or drop container capabilities.
See `man 7 capabilities` for a full list.

    cap_add:
      - ALL

    cap_drop:
      - NET_ADMIN
      - SYS_ADMIN

Using `dockerfile` together with `image` is not allowed. Attempting to do so results in an error.

### command

Override the default command.

    command: bundle exec thin -p 3000

### container_name

Specify a custom container name, rather than a generated default name.

    container_name: my-web-container

Because Docker container names must be unique, you cannot scale a service
beyond 1 container if you have specified a custom name. Attempting to do so
results in an error.

### devices

List of device mappings.  Uses the same format as the `--device` docker
client create option.

    devices:
      - "/dev/ttyUSB0:/dev/ttyUSB0"

### dns

Custom DNS servers. Can be a single value or a list.

    dns: 8.8.8.8
    dns:
      - 8.8.8.8
      - 9.9.9.9

### dns_search

Custom DNS search domains. Can be a single value or a list.

    dns_search: example.com
    dns_search:
      - dc1.example.com
      - dc2.example.com

### dockerfile

Alternate Dockerfile.

Compose will use an alternate file to build with.

    dockerfile: Dockerfile-alternate

### env_file

Add environment variables from a file. Can be a single value or a list.

If you have specified a Compose file with `docker-compose -f FILE`, paths in
`env_file` are relative to the directory that file is in.

Environment variables specified in `environment` override these values.

    env_file: .env

    env_file:
      - ./common.env
      - ./apps/web.env
      - /opt/secrets.env

Compose expects each line in an env file to be in `VAR=VAL` format. Lines
beginning with `#` (i.e. comments) are ignored, as are blank lines.

    # Set Rails/Rack environment
    RACK_ENV=development

### environment

Add environment variables. You can use either an array or a dictionary. Any
boolean values; true, false, yes no, need to be enclosed in quotes to ensure
they are not converted to True or False by the YML parser.

Environment variables with only a key are resolved to their values on the
machine Compose is running on, which can be helpful for secret or host-specific values.

    environment:
      RACK_ENV: development
      SHOW: 'true'
      SESSION_SECRET:

    environment:
      - RACK_ENV=development
      - SHOW=true
      - SESSION_SECRET

### expose

Expose ports without publishing them to the host machine - they'll only be
accessible to linked services. Only the internal port can be specified.

    expose:
     - "3000"
     - "8000"

### extends

Extend another service, in the current file or another, optionally overriding
configuration.

Here's a simple example. Suppose we have 2 files - **common.yml** and
**development.yml**. We can use `extends` to define a service in
**development.yml** which uses configuration defined in **common.yml**:

**common.yml**

    webapp:
      build: ./webapp
      environment:
        - DEBUG=false
        - SEND_EMAILS=false

**development.yml**

    web:
      extends:
        file: common.yml
        service: webapp
      ports:
        - "8000:8000"
      links:
        - db
      environment:
        - DEBUG=true
    db:
      image: postgres

Here, the `web` service in **development.yml** inherits the configuration of
the `webapp` service in **common.yml** - the `build` and `environment` keys -
and adds `ports` and `links` configuration. It overrides one of the defined
environment variables (DEBUG) with a new value, and the other one
(SEND_EMAILS) is left untouched.

The `file` key is optional, if it is not set then Compose will look for the
service within the current file.

For more on `extends`, see the [tutorial](extends.md#example) and
[reference](extends.md#reference).

### external_links

Link to containers started outside this `docker-compose.yml` or even outside
of Compose, especially for containers that provide shared or common services.
`external_links` follow semantics similar to `links` when specifying both the
container name and the link alias (`CONTAINER:ALIAS`).

    external_links:
     - redis_1
     - project_db_1:mysql
     - project_db_1:postgresql

### extra_hosts

Add hostname mappings. Use the same values as the docker client `--add-host` parameter.

    extra_hosts:
     - "somehost:162.242.195.82"
     - "otherhost:50.31.209.229"

An entry with the ip address and hostname will be created in `/etc/hosts` inside containers for this service, e.g:

    162.242.195.82  somehost
    50.31.209.229   otherhost

### image

Tag or partial image ID. Can be local or remote - Compose will attempt to
pull if it doesn't exist locally.

    image: ubuntu
    image: orchardup/postgresql
    image: a4bc65fd

### labels

Add metadata to containers using [Docker labels](http://docs.docker.com/userguide/labels-custom-metadata/). You can use either an array or a dictionary.

It's recommended that you use reverse-DNS notation to prevent your labels from conflicting with those used by other software.

    labels:
      com.example.description: "Accounting webapp"
      com.example.department: "Finance"
      com.example.label-with-empty-value: ""

    labels:
      - "com.example.description=Accounting webapp"
      - "com.example.department=Finance"
      - "com.example.label-with-empty-value"

### links

Link to containers in another service. Either specify both the service name and
the link alias (`SERVICE:ALIAS`), or just the service name (which will also be
used for the alias).

    links:
     - db
     - db:database
     - redis

An entry with the alias' name will be created in `/etc/hosts` inside containers
for this service, e.g:

    172.17.2.186  db
    172.17.2.186  database
    172.17.2.187  redis

Environment variables will also be created - see the [environment variable
reference](env.md) for details.

### log_driver

Specify a logging driver for the service's containers, as with the ``--log-driver``
option for docker run ([documented here](https://docs.docker.com/reference/logging/overview/)).

The default value is json-file.

    log_driver: "json-file"
    log_driver: "syslog"
    log_driver: "none"

> **Note:** Only the `json-file` driver makes the logs available directly from
> `docker-compose up` and `docker-compose logs`. Using any other driver will not
> print any logs.

### log_opt

Specify logging options with `log_opt` for the logging driver, as with the ``--log-opt`` option for `docker run`.

Logging options are key value pairs. An example of `syslog` options:

    log_driver: "syslog"
    log_opt:
      syslog-address: "tcp://192.168.0.42:123"

### net

Networking mode. Use the same values as the docker client `--net` parameter.

    net: "bridge"
    net: "none"
    net: "container:[name or id]"
    net: "host"

### pid

    pid: "host"

Sets the PID mode to the host PID mode.  This turns on sharing between
container and the host operating system the PID address space.  Containers
launched with this flag will be able to access and manipulate other
containers in the bare-metal machine's namespace and vise-versa.

### ports

Expose ports. Either specify both ports (`HOST:CONTAINER`), or just the container
port (a random host port will be chosen).

> **Note:** When mapping ports in the `HOST:CONTAINER` format, you may experience
> erroneous results when using a container port lower than 60, because YAML will
> parse numbers in the format `xx:yy` as sexagesimal (base 60). For this reason,
> we recommend always explicitly specifying your port mappings as strings.

    ports:
     - "3000"
     - "3000-3005"
     - "8000:8000"
     - "9090-9091:8080-8081"
     - "49100:22"
     - "127.0.0.1:8001:8001"
     - "127.0.0.1:5000-5010:5000-5010"

### security_opt

Override the default labeling scheme for each container.

      security_opt:
        - label:user:USER
        - label:role:ROLE

### volumes

Mount paths as volumes, optionally specifying a path on the host machine
(`HOST:CONTAINER`), or an access mode (`HOST:CONTAINER:ro`).

    volumes:
     - /var/lib/mysql
     - ./cache:/tmp/cache
     - ~/configs:/etc/configs/:ro

You can mount a relative path on the host, which will expand relative to
the directory of the Compose configuration file being used. Relative paths
should always begin with `.` or `..`.

> Note: No path expansion will be done if you have also specified a
> `volume_driver`.

### volumes_from

Mount all of the volumes from another service or container, optionally
specifying read-only access(``ro``) or read-write(``rw``).

    volumes_from:
     - service_name
     - container_name
     - service_name:rw

### cpu\_shares, cpuset, domainname, entrypoint, hostname, ipc, mac\_address, mem\_limit, memswap\_limit, privileged, read\_only, restart, stdin\_open, tty, user, volume\_driver, working\_dir

Each of these is a single value, analogous to its
[docker run](https://docs.docker.com/reference/run/) counterpart.

    cpu_shares: 73
    cpuset: 0,1

    entrypoint: /code/entrypoint.sh
    user: postgresql
    working_dir: /code

    domainname: foo.com
    hostname: foo
    ipc: host
    mac_address: 02:42:ac:11:65:43

    mem_limit: 1000000000
    memswap_limit: 2000000000
    privileged: true

    restart: always

    read_only: true
    stdin_open: true
    tty: true

    volume_driver: mydriver

## Variable substitution

Your configuration options can contain environment variables. Compose uses the
variable values from the shell environment in which `docker-compose` is run. For
example, suppose the shell contains `POSTGRES_VERSION=9.3` and you supply this
configuration:

    db:
      image: "postgres:${POSTGRES_VERSION}"

When you run `docker-compose up` with this configuration, Compose looks for the
`POSTGRES_VERSION` environment variable in the shell and substitutes its value
in. For this example, Compose resolves the `image` to `postgres:9.3` before
running the configuration.

If an environment variable is not set, Compose substitutes with an empty
string. In the example above, if `POSTGRES_VERSION` is not set, the value for
the `image` option is `postgres:`.

Both `$VARIABLE` and `${VARIABLE}` syntax are supported. Extended shell-style
features, such as `${VARIABLE-default}` and `${VARIABLE/foo/bar}`, are not
supported.

If you need to put a literal dollar sign in a configuration value, use a double
dollar sign (`$$`).


## Compose documentation

- [User guide](index.md)
- [Installing Compose](install.md)
- [Get started with Django](django.md)
- [Get started with Rails](rails.md)
- [Get started with WordPress](wordpress.md)
- [Command line reference](./reference/index.md)
- [Compose environment variables](env.md)
- [Compose command line completion](completion.md)
