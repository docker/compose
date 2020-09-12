# Installing the Docker ACI Integration CLI on Linux (Beta)

This CLI adds support for running and managing containers on Azure Container
Instances (ACI).

> :warning: **This CLI is in beta**: The installation process, commands, and
> flags will change in future releases.

## Prerequisites

* [Docker 19.03 or later](https://docs.docker.com/get-docker/)

## Install script

You can install the new CLI using the install script:

```console
curl -L https://github.com/docker/aci-integration-beta/releases/download/v0.1.4/install.sh | sh
```

## Manual install

You can download the Docker ACI Integration CLI using the following command:

```console
curl -Lo docker-aci https://github.com/docker/aci-integration-beta/releases/download/v0.1.4/docker-linux-amd64
```

You will then need to make it executable:

```console
chmod +x docker-aci
```

To enable using the local Docker Engine and to use existing Docker contexts, you
will need to have the existing Docker CLI as `com.docker.cli` somewhere in your
`PATH`. You can do this by creating a symbolic link from the existing Docker
CLI.

```console
ln -s /path/to/existing/docker /directory/in/PATH/com.docker.cli
```

> **Note**: The `PATH` environment variable is a colon separated list of
> directories with priority from left to right. You can view it using
> `echo $PATH`. You can find the path to the existing Docker CLI using
> `which docker`. You may need root permissions to make this link.

On a fresh install of Ubuntu 20.04 with Docker Engine
[already installed](https://docs.docker.com/engine/install/ubuntu/):

```console
$ echo $PATH
/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin
$ which docker
/usr/bin/docker
$ sudo ln -s /usr/bin/docker /usr/local/bin/com.docker.cli
```

You can verify that this is working by checking that the new CLI works with the
default context:

```console
$ ./docker-aci --context default ps
CONTAINER ID        IMAGE               COMMAND             CREATED             STATUS              PORTS               NAMES
$ echo $?
0
```

To make this CLI with ACI integration your default Docker CLI, you must move it
to a directory in your `PATH` with higher priority than the existing Docker CLI.

Again on a fresh Ubuntu 20.04:

```console
$ which docker
/usr/bin/docker
$ echo $PATH
/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin
$ sudo mv docker-aci /usr/local/bin/docker
$ which docker
/usr/local/bin/docker
$ docker version
...
 Cloud integration  0.1.6
...
```

## Uninstall

To remove this CLI, you need to remove the binary you downloaded and
`com.docker.cli` from your `PATH`. If you installed using the script, this can
be done as follows:

```console
sudo rm /usr/local/bin/docker /usr/local/bin/com.docker.cli
```
