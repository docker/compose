<!--[metadata]>
+++
title = "Compose CLI reference"
description = "Compose CLI reference"
keywords = ["fig, composition, compose, docker, orchestration, cli,  reference"]
[menu.main]
identifier = "smn_install_compose"
parent = "smn_compose_ref"	
+++
<![end-metadata]-->


# Compose CLI reference

Most Docker Compose commands are run against one or more services. If
the service is not specified, the command will apply to all services.

For full usage information, run `docker-compose [COMMAND] --help`.

## Commands

### build

Builds or rebuilds services.

Services are built once and then tagged as `project_service`, e.g.,
`composetest_db`. If you change a service's Dockerfile or the contents of its
build directory, run `docker-compose build` to rebuild it.

### help

Displays help and usage instructions for a command.

### kill

Forces running containers to stop by sending a `SIGKILL` signal. Optionally the
signal can be passed, for example:

    $ docker-compose kill -s SIGINT

### logs

Displays log output from services.

### port

Prints the public port for a port binding

### ps

Lists containers.

### pull

Pulls service images.

### restart

Restarts services.

### rm

Removes stopped service containers.


### run

Runs a one-off command on a service.

For example,

    $ docker-compose run web python manage.py shell

will start the `web` service and then run `manage.py shell` in python.
Note that by default, linked services will also be started, unless they are
already running.

One-off commands are started in new containers with the same configuration as a
normal container for that service, so volumes, links, etc will all be created as
expected. When using `run`, there are two differences from bringing up a
container normally:

1. the command will be overridden with the one specified. So, if you run
`docker-compose run web bash`, the container's web command (which could default
to, e.g., `python app.py`) will be overridden to `bash`

2. by default no ports will be created in case they collide with already opened
ports.

Links are also created between one-off commands and the other containers which
are part of that service. So, for example, you could run:

    $ docker-compose run db psql -h db -U docker

This would open up an interactive PostgreSQL shell for the linked `db` container
(which would get created or started as needed).

If you do not want linked containers to start when running the one-off command,
specify the `--no-deps` flag:

    $ docker-compose run --no-deps web python manage.py shell

Similarly, if you do want the service's ports to be created and mapped to the
host, specify the `--service-ports` flag:

    $ docker-compose run --service-ports web python manage.py shell


### scale

Sets the number of containers to run for a service.

Numbers are specified as arguments in the form `service=num`. For example:

    $ docker-compose scale web=2 worker=3

### start

Starts existing containers for a service.

### stop

Stops running containers without removing them. They can be started again with
`docker-compose start`.

### up

Builds, (re)creates, starts, and attaches to containers for a service.

Linked services will be started, unless they are already running.

By default, `docker-compose up` will aggregate the output of each container and,
when it exits, all containers will be stopped. Running `docker-compose up -d`,
will start the containers in the background and leave them running.

By default, if there are existing containers for a service, `docker-compose up` will stop and recreate them (preserving mounted volumes with [volumes-from]), so that changes in `docker-compose.yml` are picked up. If you do not want containers stopped and recreated, use `docker-compose up --no-recreate`. This will still start any stopped containers, if needed.

[volumes-from]: http://docs.docker.io/en/latest/use/working_with_volumes/

## Options

### --verbose

 Shows more output

### -v, --version

 Prints version and exits

### -f, --file FILE

 Specify what file to read configuration from. If not provided, Compose will look
 for `docker-compose.yml` in the current working directory, and then each parent
 directory successively, until found.


### -p, --project-name NAME

 Specifies an alternate project name (default: current directory name)


## Environment Variables

Several environment variables are available for you to configure Compose's behaviour.

Variables starting with `DOCKER_` are the same as those used to configure the
Docker command-line client. If you're using boot2docker, `eval "$(boot2docker shellinit)"`
will set them to their correct values.

### COMPOSE\_PROJECT\_NAME

Sets the project name, which is prepended to the name of every container started by Compose. Defaults to the `basename` of the current working directory.

### COMPOSE\_FILE

Specify what file to read configuration from. If not provided, Compose will look
for `docker-compose.yml` in the current working directory, and then each parent
directory successively, until found.

### DOCKER\_HOST

Sets the URL of the docker daemon. As with the Docker client, defaults to `unix:///var/run/docker.sock`.

### DOCKER\_TLS\_VERIFY

When set to anything other than an empty string, enables TLS communication with
the daemon.

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
