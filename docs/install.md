---
layout: default
title: Installing Fig
---

Installing Fig
==============

First, install Docker version 1.0 or greater. If you're on OS X, you can use [docker-osx](https://github.com/noplay/docker-osx):

    $ curl https://raw.githubusercontent.com/noplay/docker-osx/1.1.1/docker-osx > /usr/local/bin/docker-osx
    $ chmod +x /usr/local/bin/docker-osx
    $ docker-osx shell

Docker has guides for [Ubuntu](http://docs.docker.io/en/latest/installation/ubuntulinux/) and [other platforms](http://docs.docker.io/en/latest/installation/) in their documentation.

Next, install Fig:

    curl -L https://github.com/docker/fig/releases/download/0.5.2/fig-`uname -s`-`uname -m` > /usr/local/bin/fig; chmod +x /usr/local/bin/fig

Releases are available for OS X and 64-bit Linux. Fig is also available as a Python package if you're on another platform (or if you prefer that sort of thing):

    $ sudo pip install -U fig

That should be all you need! Run `fig --version` to see if it worked.
