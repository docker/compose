<!--[metadata]>
+++
title = "Docker Compose"
description = "How to install Docker Compose"
keywords = ["compose, orchestration, install, installation, docker, documentation"]
[menu.main]
parent="mn_install"
weight=4
+++
<![end-metadata]-->


# Install Docker Compose

To install Compose, you'll need to install Docker first. You'll then install
Compose with a `curl` command. 

## Install Docker

First, install Docker version 1.7.1 or greater:

- [Instructions for Mac OS X](http://docs.docker.com/installation/mac/)
- [Instructions for Ubuntu](http://docs.docker.com/installation/ubuntulinux/)
- [Instructions for other systems](http://docs.docker.com/installation/)

## Install Compose

To install Compose, run the following commands:

    curl -L https://github.com/docker/compose/releases/download/1.3.3/docker-compose-`uname -s`-`uname -m` > /usr/local/bin/docker-compose
    chmod +x /usr/local/bin/docker-compose

> Note: If you get a "Permission denied" error, your `/usr/local/bin` directory probably isn't writable and you'll need to install Compose as the superuser. Run `sudo -i`, then the two commands above, then `exit`.

Optionally, you can also install [command completion](completion.md) for the
bash and zsh shell.

> **Note:** Some older Mac OS X CPU architectures are incompatible with the binary. If you receive an "Illegal instruction: 4" error after installing, you should install using the `pip` command instead.

Compose is available for OS X and 64-bit Linux. If you're on another platform,
Compose can also be installed as a Python package:

    $ sudo pip install -U docker-compose

No further steps are required; Compose should now be successfully installed.
You can test the installation by running `docker-compose --version`.

### Upgrading

If you're coming from Compose 1.2 or earlier, you'll need to remove or migrate your existing containers after upgrading Compose. This is because, as of version 1.3, Compose uses Docker labels to keep track of containers, and so they need to be recreated with labels added.

If Compose detects containers that were created without labels, it will refuse to run so that you don't end up with two sets of them. If you want to keep using your existing containers (for example, because they have data volumes you want to preserve) you can migrate them with the following command:

    docker-compose migrate-to-labels

Alternatively, if you're not worried about keeping them, you can remove them - Compose will just create new ones.

    docker rm -f myapp_web_1 myapp_db_1 ...


## Uninstallation

To uninstall Docker Compose if you installed using `curl`:

    $ rm /usr/local/bin/docker-compose


To uninstall Docker Compose if you installed using `pip`:

    $ pip uninstall docker-compose
    
> Note: If you get a "Permission denied" error using either of the above methods, you probably do not have the proper permissions to remove `docker-compose`.  To force the removal, prepend `sudo` to either of the above commands and run again.


## Compose documentation

- [User guide](/)
- [Get started with Django](django.md)
- [Get started with Rails](rails.md)
- [Get started with Wordpress](wordpress.md)
- [Command line reference](cli.md)
- [Yaml file reference](yml.md)
- [Compose environment variables](env.md)
- [Compose command line completion](completion.md)
