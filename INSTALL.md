# Mac and Windows installation

The Compose CLI is built into Docker Desktop Edge and Stable.
You can download it from these links:
- [macOS](https://hub.docker.com/editions/community/docker-ce-desktop-mac)
- [Windows](https://hub.docker.com/editions/community/docker-ce-desktop-windows)

# Ubuntu Linux installation

The Linux installation script and manual install instructions have been tested
with a fresh install of Ubuntu 20.04.

## Prerequisites

* [Docker 19.03 or later](https://docs.docker.com/engine/install/)

## Install script

You can install the Compose CLI using the install script:

```console
curl -L https://raw.githubusercontent.com/docker/compose-cli/main/scripts/install_linux.sh | sh
```

## Manual install

You can download the Compose CLI from [latest release](https://github.com/docker/compose-cli/releases/latest).

You will then need to extract it and make it executable:

```console
$ tar xzf docker-linux-amd64.tar.gz
$ chmod +x docker/docker
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
$ ./docker/docker --context default ps
CONTAINER ID        IMAGE               COMMAND             CREATED             STATUS              PORTS               NAMES
$ echo $?
0
```

To make the Compose CLI your default Docker CLI, you must move it to a directory
in your `PATH` with higher priority than the existing Docker CLI.

Again on a fresh Ubuntu 20.04:

```console
$ which docker
/usr/bin/docker
$ echo $PATH
/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin
$ sudo mv docker/docker /usr/local/bin/docker
$ which docker
/usr/local/bin/docker
$ docker version
...
 Cloud integration  0.1.6
...
```

# Uninstall

To remove this CLI, you need to remove the binary you downloaded and
`com.docker.cli` from your `PATH`. If you installed using the script, this can
be done as follows:

```console
sudo rm /usr/local/bin/docker /usr/local/bin/com.docker.cli
```

# Testing the install script

To test the install script, from a machine with docker:

```console
docker build -t testclilinux -f scripts/Dockerfile-testInstall scripts
```