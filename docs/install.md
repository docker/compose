---
layout: default
title: Installing Fig
---

Installing Fig
==============

First, install Docker version 1.3 or greater.

If you're on OS X, you can use the [OS X installer](https://docs.docker.com/installation/mac/). You'll also need to set an environment variable to point at the Boot2Docker virtual machine:

    $ export DOCKER_HOST=tcp://`boot2docker ip`:2375

If you want this to persist across shell sessions, you can add it to your `~/.bashrc` file.

There are also guides for [Ubuntu](https://docs.docker.com/installation/ubuntulinux/) and [other platforms](https://docs.docker.com/installation/) in Docker’s documentation.

Next, install Fig:

    curl -L https://github.com/docker/fig/releases/download/0.5.2/fig-`uname -s`-`uname -m` > /usr/local/bin/fig; chmod +x /usr/local/bin/fig

Releases are available for OS X and 64-bit Linux. Fig is also available as a Python package if you're on another platform (or if you prefer that sort of thing):

    $ sudo pip install -U fig
    
If, during the install of the PyYAML dependency, you get the following error:

```
ext/_yaml.c:4:20: fatal error: Python.h: No such file or directory
```

you should install the Python development package for your distro (`apt-get install python-dev`, `yum install python-devel`, etc.)

That should be all you need! Run `fig --version` to see if it worked.
