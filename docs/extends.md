<!--[metadata]>
+++
title = "Extending Services in Compose"
description = "How to use Docker Compose's extends keyword to share configuration between files and projects"
keywords = ["fig, composition, compose, docker, orchestration, documentation, docs"]
[menu.main]
parent="workw_compose"
weight=20
+++
<![end-metadata]-->


# Extending services and Compose files

Compose supports two methods of sharing common configuration:

1. Extending an entire Compose file by
   [using multiple Compose files](#multiple-compose-files)
2. Extending individual services with [the `extends` field](#extending-services)


## Multiple Compose files

Using multiple Compose files enables you to customize a Compose application
for different environments or different workflows.

### Understanding multiple Compose files

By default, Compose reads two files, a `docker-compose.yml` and an optional
`docker-compose.override.yml` file. By convention, the `docker-compose.yml`
contains your base configuration. The override file, as its name implies, can
contain configuration overrides for existing services or entirely new
services.

If a service is defined in both files Compose merges the configurations using
the rules described in [Adding and overriding
configuration](#adding-and-overriding-configuration).

To use multiple override files, or an override file with a different name, you
can use the `-f` option to specify the list of files. Compose merges files in
the order they're specified on the command line. See the [`docker-compose`
command reference](./reference/overview.md) for more information about
using `-f`.

When you use multiple configuration files, you must make sure all paths in the
files are relative to the base Compose file (the first Compose file specified
with `-f`). This is required because override files need not be valid
Compose files. Override files can contain small fragments of configuration.
Tracking which fragment of a service is relative to which path is difficult and
confusing, so to keep paths easier to understand, all paths must be defined
relative to the base file.

### Example use case

In this section are two common use cases for multiple compose files: changing a
Compose app for different environments, and running administrative tasks
against a Compose app.

#### Different environments

A common use case for multiple files is changing a development Compose app
for a production-like environment (which may be production, staging or CI).
To support these differences, you can split your Compose configuration into
a few different files:

Start with a base file that defines the canonical configuration for the
services.

**docker-compose.yml**

    web:
      image: example/my_web_app:latest
      links:
        - db
        - cache

    db:
      image: postgres:latest

    cache:
      image: redis:latest

In this example the development configuration exposes some ports to the
host, mounts our code as a volume, and builds the web image.

**docker-compose.override.yml**


    web:
      build: .
      volumes:
        - '.:/code'
      ports:
        - 8883:80
      environment:
        DEBUG: 'true'

    db:
      command: '-d'
      ports:
        - 5432:5432

    cache:
      ports:
        - 6379:6379

When you run `docker-compose up` it reads the overrides automatically.

Now, it would be nice to use this Compose app in a production environment. So,
create another override file (which might be stored in a different git
repo or managed by a different team).

**docker-compose.prod.yml**

    web:
      ports:
        - 80:80
      environment:
        PRODUCTION: 'true'

    cache:
      environment:
        TTL: '500'

To deploy with this production Compose file you can run

    docker-compose -f docker-compose.yml -f docker-compose.prod.yml up -d

This deploys all three services using the configuration in
`docker-compose.yml` and `docker-compose.prod.yml` (but not the
dev configuration in `docker-compose.override.yml`).


See [production](production.md) for more information about Compose in
production.

#### Administrative tasks

Another common use case is running adhoc or administrative tasks against one
or more services in a Compose app. This example demonstrates running a
database backup.

Start with a **docker-compose.yml**.

    web:
      image: example/my_web_app:latest
      links:
        - db

    db:
      image: postgres:latest

In a **docker-compose.admin.yml** add a new service to run the database
export or backup.

    dbadmin:
      build: database_admin/
      links:
        - db

To start a normal environment run `docker-compose up -d`. To run a database
backup, include the `docker-compose.admin.yml` as well.

    docker-compose -f docker-compose.yml -f docker-compose.admin.yml \
        run dbadmin db-backup


## Extending services

Docker Compose's `extends` keyword enables sharing of common configurations
among different files, or even different projects entirely. Extending services
is useful if you have several services that reuse a common set of configuration
options. Using `extends` you can define a common set of service options in one
place and refer to it from anywhere.

> **Note:** `links`, `volumes_from`, and `depends_on` are never shared between
> services using >`extends`. These exceptions exist to avoid
> implicit dependencies&mdash;you always define `links` and `volumes_from`
> locally. This ensures dependencies between services are clearly visible when
> reading the current file. Defining these locally also ensures changes to the
> referenced file don't result in breakage.

### Understand the extends configuration

When defining any service in `docker-compose.yml`, you can declare that you are
extending another service like this:

    web:
      extends:
        file: common-services.yml
        service: webapp

This instructs Compose to re-use the configuration for the `webapp` service
defined in the `common-services.yml` file. Suppose that `common-services.yml`
looks like this:

    webapp:
      build: .
      ports:
        - "8000:8000"
      volumes:
        - "/data"

In this case, you'll get exactly the same result as if you wrote
`docker-compose.yml` with the same `build`, `ports` and `volumes` configuration
values defined directly under `web`.

You can go further and define (or re-define) configuration locally in
`docker-compose.yml`:

    web:
      extends:
        file: common-services.yml
        service: webapp
      environment:
        - DEBUG=1
      cpu_shares: 5

    important_web:
      extends: web
      cpu_shares: 10

You can also write other services and link your `web` service to them:

    web:
      extends:
        file: common-services.yml
        service: webapp
      environment:
        - DEBUG=1
      cpu_shares: 5
      links:
        - db
    db:
      image: postgres

### Example use case

Extending an individual service is useful when you have multiple services that
have a common configuration.  The example below is a Compose app with
two services: a web application and a queue worker. Both services use the same
codebase and share many configuration options.

In a **common.yml** we define the common configuration:

    app:
      build: .
      environment:
        CONFIG_FILE_PATH: /code/config
        API_KEY: xxxyyy
      cpu_shares: 5

In a **docker-compose.yml** we define the concrete services which use the
common configuration:

    webapp:
      extends:
        file: common.yml
        service: app
      command: /code/run_web_app
      ports:
        - 8080:8080
      links:
        - queue
        - db

    queue_worker:
      extends:
        file: common.yml
        service: app
      command: /code/run_worker
      links:
        - queue

## Adding and overriding configuration

Compose copies configurations from the original service over to the local one.
If a configuration option is defined in both the original service the local
service, the local value *replaces* or *extends* the original value.

For single-value options like `image`, `command` or `mem_limit`, the new value
replaces the old value.

    # original service
    command: python app.py

    # local service
    command: python otherapp.py

    # result
    command: python otherapp.py

> **Note:** In the case of `build` and `image`, when using
> [version 1 of the Compose file format](compose-file.md#version-1), using one
> option in the local service causes Compose to discard the other option if it
> was defined in the original service.
>
> For example, if the original service defines `image: webapp` and the
> local service defines `build: .` then the resulting service will have
> `build: .` and no `image` option.
>
> This is because `build` and `image` cannot be used together in a version 1
> file.

For the **multi-value options** `ports`, `expose`, `external_links`, `dns`,
`dns_search`, and `tmpfs`, Compose concatenates both sets of values:

    # original service
    expose:
      - "3000"

    # local service
    expose:
      - "4000"
      - "5000"

    # result
    expose:
      - "3000"
      - "4000"
      - "5000"

In the case of `environment`, `labels`, `volumes` and `devices`, Compose
"merges" entries together with locally-defined values taking precedence:

    # original service
    environment:
      - FOO=original
      - BAR=original

    # local service
    environment:
      - BAR=local
      - BAZ=local

    # result
    environment:
      - FOO=original
      - BAR=local
      - BAZ=local




## Compose documentation

- [User guide](index.md)
- [Installing Compose](install.md)
- [Getting Started](gettingstarted.md)
- [Get started with Django](django.md)
- [Get started with Rails](rails.md)
- [Get started with WordPress](wordpress.md)
- [Command line reference](./reference/index.md)
- [Compose file reference](compose-file.md)
