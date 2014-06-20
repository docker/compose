---
layout: default
title: Installing Fig
---

Installing Fig
==============

First, [install Docker](https://docs.docker.com/installation/) version 1.0.0. 

Next, install Fig. On OS X:

    $ curl -L https://github.com/orchardup/fig/releases/download/0.4.2/darwin > /usr/local/bin/fig
    $ chmod +x /usr/local/bin/fig

On 64-bit Linux:

    $ curl -L https://github.com/orchardup/fig/releases/download/0.4.2/linux > /usr/local/bin/fig
    $ chmod +x /usr/local/bin/fig

Fig is also available as a Python package if you're on another platform (or if you prefer that sort of thing):

    $ sudo pip install -U fig

That should be all you need! Run `fig --version` to see if it worked.
