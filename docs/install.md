---
layout: default
title: Installing Compose
---

Installing Compose
==============

First, install Docker version 1.3 or greater.

If you're on OS X, you can use the [OS X installer](https://docs.docker.com/installation/mac/) to install both Docker and boot2docker. Once boot2docker is running, set the environment variables that'll configure Docker and Compose to talk to it:

    $(boot2docker shellinit)

To persist the environment variables across shell sessions, you can add that line to your `~/.bashrc` file.

There are also guides for [Ubuntu](https://docs.docker.com/installation/ubuntulinux/) and [other platforms](https://docs.docker.com/installation/) in Docker’s documentation.

Next, install Compose:

    curl -L https://github.com/docker/docker-compose/releases/download/1.0.1/docker-compose-`uname -s`-`uname -m` > /usr/local/bin/docker-compose; chmod +x /usr/local/bin/docker-compose

Optionally, install [command completion](completion.html) for the bash shell.

Releases are available for OS X and 64-bit Linux. Compose is also available as a Python package if you're on another platform (or if you prefer that sort of thing):

    $ sudo pip install -U docker-compose

That should be all you need! Run `docker-compose --version` to see if it worked.
