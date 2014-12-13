---
layout: default
title: Installing Fig
---

Installing Fig
==============

First, install Docker version 1.3 or greater.

If you're on OS X, you can use the [OS X installer](https://docs.docker.com/installation/mac/) to install both Docker and boot2docker. Once boot2docker is running, set the environment variables that'll configure Docker and Fig to talk to it:

    $(boot2docker shellinit)

To persist the environment variables across shell sessions, you can add that line to your `~/.bashrc` file.

There are also guides for [Ubuntu](https://docs.docker.com/installation/ubuntulinux/) and [other platforms](https://docs.docker.com/installation/) in Docker’s documentation.

Next, install Fig:

    curl -L https://github.com/docker/fig/releases/download/1.0.1/fig-`uname -s`-`uname -m` > /usr/local/bin/fig; chmod +x /usr/local/bin/fig

Releases are available for OS X and 64-bit Linux. Fig is also available as a Python package if you're on another platform (or if you prefer that sort of thing):

    $ sudo pip install -U fig

That should be all you need! Run `fig --version` to see if it worked.

Development version
-------------------

To install the very latest development version of fig use
python-pip to install directly from github


    sudo pip install git+https://github.com/docker/fig.git@master#egg=fig-master


or you can build a binary yourself

    git clone https://github.com/docker/fig.git
    cd fig
    # set the version to .dev
    sed -i -e "s/__version__ = '\(.*\)'/__version__ = '\1.dev'/" fig/__init__.py
    ./script/build-osx # or ./script/build-linux
    ls dist/ 
