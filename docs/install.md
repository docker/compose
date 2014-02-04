---
layout: default
title: Installing Fig
---

Installing Fig
==============

First, install Docker. If you're on OS X, you can use [docker-osx](https://github.com/noplay/docker-osx):

    $ curl https://raw.github.com/noplay/docker-osx/4e929004727f73497da99a8d7ec55b2381ecd74c/docker-osx > /usr/local/bin/docker-osx
    $ chmod +x /usr/local/bin/docker-osx
    $ docker-osx shell

Docker has guides for [Ubuntu](http://docs.docker.io/en/latest/installation/ubuntulinux/) and [other platforms](http://docs.docker.io/en/latest/installation/) in their documentation.

Next, install Fig:

    $ sudo pip install -U fig

(This command also upgrades Fig when we release a new version. If you don’t have pip installed, try `brew install python` or `apt-get install python-pip`.)

That should be all you need! Run `fig --version` to see if it worked.
