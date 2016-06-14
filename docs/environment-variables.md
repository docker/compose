<!--[metadata]>
+++
title = "Environment variables in Compose"
description = "How to set, use and manage environment variables in Compose"
keywords = ["fig, composition, compose, docker, orchestration, environment, variables, env file"]
[menu.main]
parent = "workw_compose"
weight=10
+++
<![end-metadata]-->

# Environment variables in Compose

There are multiple parts of Compose that deal with environment variables in one sense or another. This page should help you find the information you need.


## Substituting environment variables in Compose files

It's possible to use environment variables in your shell to populate values inside a Compose file:

    web:
      image: "webapp:${TAG}"

For more information, see the [Variable substitution](compose-file.md#variable-substitution) section in the Compose file reference.


## Setting environment variables in containers

You can set environment variables in a service's containers with the ['environment' key](compose-file.md#environment), just like with `docker run -e VARIABLE=VALUE ...`:

    web:
      environment:
        - DEBUG=1


## Passing environment variables through to containers

You can pass environment variables from your shell straight through to a service's containers with the ['environment' key](compose-file.md#environment) by not giving them a value, just like with `docker run -e VARIABLE ...`:

    web:
      environment:
        - DEBUG

The value of the `DEBUG` variable in the container will be taken from the value for the same variable in the shell in which Compose is run.


## The “env_file” configuration option

You can pass multiple environment variables from an external file through to a service's containers with the ['env_file' option](compose-file.md#env-file), just like with `docker run --env-file=FILE ...`:

    web:
      env_file:
        - web-variables.env


## Setting environment variables with 'docker-compose run'

Just like with `docker run -e`, you can set environment variables on a one-off container with `docker-compose run -e`:

    $ docker-compose run -e DEBUG=1 web python console.py

You can also pass a variable through from the shell by not giving it a value:

    $ docker-compose run -e DEBUG web python console.py

The value of the `DEBUG` variable in the container will be taken from the value for the same variable in the shell in which Compose is run.


## The “.env” file

You can set default values for any environment variables referenced in the Compose file, or used to configure Compose, in an [environment file](env-file.md) named `.env`:

    $ cat .env
    TAG=v1.5

    $ cat docker-compose.yml
    version: '2.0'
    services:
      web:
        image: "webapp:${TAG}"

When you run `docker-compose up`, the `web` service defined above uses the image `webapp:v1.5`. You can verify this with the [config command](reference/config.md), which prints your resolved application config to the terminal:

    $ docker-compose config
    version: '2.0'
    services:
      web:
        image: 'webapp:v1.5'

Values in the shell take precedence over those specified in the `.env` file. If you set `TAG` to a different value in your shell, the substitution in `image` uses that instead:

    $ export TAG=v2.0

    $ docker-compose config
    version: '2.0'
    services:
      web:
        image: 'webapp:v2.0'

## Configuring Compose using environment variables

Several environment variables are available for you to configure the Docker Compose command-line behaviour. They begin with `COMPOSE_` or `DOCKER_`, and are documented in [CLI Environment Variables](reference/envvars.md).


## Environment variables created by links

When using the ['links' option](compose-file.md#links) in a [v1 Compose file](compose-file.md#version-1), environment variables will be created for each link. They are documented in the [Link environment variables reference](link-env-deprecated.md). Please note, however, that these variables are deprecated - you should just use the link alias as a hostname instead.
