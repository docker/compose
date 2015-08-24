<!--[metadata]>
+++
title = "Introduction to the CLI"
description = "Introduction to the CLI"
keywords = ["fig, composition, compose, docker, orchestration, cli,  reference"]
[menu.main]
parent = "smn_compose_cli"
weight=-2
+++
<![end-metadata]-->


# Introduction to the CLI

This section describes the subcommands you can use with the `docker-compose` command.  You can run subcommand against one or more services. To run against a specific service, you supply the service name from your compose configuration. If you do not specify the service name, the command runs against all the services in your configuration.

## Environment Variables

Several environment variables are available for you to configure the Docker Compose command-line behaviour.

Variables starting with `DOCKER_` are the same as those used to configure the
Docker command-line client. If you're using `docker-machine`, then the `eval "$(docker-machine env my-docker-vm)"` command should set them to their correct values. (In this example, `my-docker-vm` is the name of a machine you created.)

### COMPOSE\_PROJECT\_NAME

Sets the project name. This value is prepended along with the service name to the container container on start up. For example, if you project name is `myapp` and it includes two services `db` and `web` then compose starts containers named  `myapp_db_1` and `myapp_web_1` respectively.

Setting this is optional. If you do not set this, the `COMPOSE_PROJECT_NAME` defaults to the `basename` of the current working directory.

### COMPOSE\_FILE

Specify the file containing the compose configuration. If not provided, Compose looks for a file named  `docker-compose.yml` in the current directory and then each parent directory in succession until a file by that name is found.

### DOCKER\_HOST

Sets the URL of the `docker` daemon. As with the Docker client, defaults to `unix:///var/run/docker.sock`.

### DOCKER\_TLS\_VERIFY

When set to anything other than an empty string, enables TLS communication with
the `docker` daemon.

### DOCKER\_CERT\_PATH

Configures the path to the `ca.pem`, `cert.pem`, and `key.pem` files used for TLS verification. Defaults to `~/.docker`.







## Compose documentation

- [User guide](/)
- [Installing Compose](install.md)
- [Get started with Django](django.md)
- [Get started with Rails](rails.md)
- [Get started with Wordpress](wordpress.md)
- [Yaml file reference](yml.md)
- [Compose environment variables](env.md)
- [Compose command line completion](completion.md)
