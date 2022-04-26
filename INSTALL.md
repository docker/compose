# Install Docker Compose

This page contains information on how to install Docker Compose. You can run Compose on macOS, Windows, and 64-bit Linux.

> ⚠️ The installation instructions on this page will help you to install Compose v1 which is a deprecated version. We recommend that you use the [latest version of Docker Compose](https://docs.docker.com/compose/install/).

## Prerequisites

Docker Compose relies on Docker Engine for any meaningful work, so make sure you
have Docker Engine installed either locally or remote, depending on your setup.


- Install
  [Docker Engine](https://docs.docker.com/engine/install/#server)
  for your OS and then come back here for
  instructions on installing the Python version of Compose.

- To run Compose as a non-root user, see [Manage Docker as a non-root user](https://docs.docker.com/engine/install/linux-postinstall/).

## Install Compose


Follow the instructions below to install Compose using the `pip`
Python package manager or to install Compose as a container.

> Install a different version
>
> The instructions below outline installation of the current stable release
> (**v1.29.2**) of Compose. To install a different version of
> Compose, replace the given release number with the one that you want. For instructions to install Compose 2.x.x on Linux, see [Install Compose 2.x.x on Linux](https://docs.docker.com/compose/install/#install-compose-on-linux-systems).
>
> Compose releases are also listed and available for direct download on the
> [Compose repository release page on GitHub](https://github.com/docker/compose/releases).
> To install a **pre-release** of Compose, refer to the [install pre-release builds](#install-pre-release-builds)
> section.

- [Install using pip](#install-using-pip)
- [Install as a container](#install-as-a-container)

#### Install using pip

> For `alpine`, the following dependency packages are needed:
> `py-pip`, `python3-dev`, `libffi-dev`, `openssl-dev`, `gcc`, `libc-dev`, `rust`, `cargo`, and `make`.
{: .important}

You can install Compose from
[pypi](https://pypi.python.org/pypi/docker-compose) using `pip`. If you install
using `pip`, we recommend that you use a
[virtualenv](https://virtualenv.pypa.io/en/latest/) because many operating
systems have python system packages that conflict with docker-compose
dependencies. See the [virtualenv
tutorial](https://docs.python-guide.org/dev/virtualenvs/) to get
started.

```console
$ pip3 install docker-compose
```

If you are not using virtualenv,

```console
$ sudo pip install docker-compose
```

> pip version 6.0 or greater is required.

#### Install as a container

You can also run Compose inside a container, from a small bash script wrapper. To
install Compose as a container run this command:

```console
$ sudo curl -L --fail https://github.com/docker/compose/releases/download/1.29.2/run.sh -o /usr/local/bin/docker-compose
$ sudo chmod +x /usr/local/bin/docker-compose
```


### Install pre-release builds

If you're interested in trying out a pre-release build, you can download release
candidates from the [Compose repository release page on GitHub](https://github.com/docker/compose/releases).
Follow the instructions from the link, which involves running the `curl` command
in your terminal to download the binaries.

Pre-releases built from the "master" branch are also available for download at
[https://dl.bintray.com/docker-compose/master/](https://dl.bintray.com/docker-compose/master/).

> Pre-release builds allow you to try out new features before they are released,
> but may be less stable.

----

## Upgrading

If you're upgrading from Compose 1.2 or earlier, remove or
migrate your existing containers after upgrading Compose. This is because, as of
version 1.3, Compose uses Docker labels to keep track of containers, and your
containers need to be recreated to add the labels.

If Compose detects containers that were created without labels, it refuses
to run, so that you don't end up with two sets of them. If you want to keep using
your existing containers (for example, because they have data volumes you want
to preserve), you can use Compose 1.5.x to migrate them with the following
command:

```console
$ docker-compose migrate-to-labels
```

Alternatively, if you're not worried about keeping them, you can remove them.
Compose just creates new ones.

```console
$ docker container rm -f -v myapp_web_1 myapp_db_1 ...
```

## Uninstall

To uninstall Docker Compose if you installed using `curl`:

```console
$ sudo rm /usr/local/bin/docker-compose
```

To uninstall Docker Compose if you installed using `pip`:

```console
$ pip uninstall docker-compose
```

> Got a "Permission denied" error?
>
> If you get a "Permission denied" error using either of the above
> methods, you probably do not have the proper permissions to remove
> `docker-compose`. To force the removal, prepend `sudo` to either of the above
> commands and run again.
