Fig
===

[![Build Status](https://travis-ci.org/orchardup/fig.png?branch=master)](https://travis-ci.org/orchardup/fig)
[![PyPI version](https://badge.fury.io/py/fig.png)](http://badge.fury.io/py/fig)

Fast, isolated development environments using Docker.

Define your app's environment with Docker so it can be reproduced anywhere:

    FROM orchardup/python:2.7
    ADD . /code
    WORKDIR /code
    RUN pip install -r requirements.txt
    CMD python app.py

Define the services that make up your app so they can be run together in an isolated environment:

```yaml
web:
  build: .
  links:
   - db
  ports:
   - "8000:8000"
   - "49100:22"
db:
  image: orchardup/postgresql
```

(No more installing Postgres on your laptop!)

Then type `fig up`, and Fig will start and run your entire app:

![example fig run](https://orchardup.com/static/images/fig-example-large.f96065fc9e22.gif)

There are commands to:

 - start, stop and rebuild services
 - view the status of running services
 - tail running services' log output
 - run a one-off command on a service

Fig is a project from [Orchard](https://orchardup.com), a Docker hosting service. [Follow us on Twitter](https://twitter.com/orchardup) to keep up to date with Fig and other Docker news.

Installation and documentation
------------------------------

Full documentation is available on [Fig's website](http://orchardup.github.io/fig/).

Running the test suite
----------------------

    $ script/test

Building OS X binaries
---------------------

    $ script/build-osx

Note that this only works on Mountain Lion, not Mavericks, due to a [bug in PyInstaller](http://www.pyinstaller.org/ticket/807).

Contributing to Fig
-------------------

If you're looking contribute to [Fig](http://orchardup.github.io/fig/)
but you're new to the project or maybe even to Python, here are the steps
that should get you started.

1. Fork [https://github.com/orchardup/fig](https://github.com/orchardup/fig) to your username. kvz in this example.
1. Clone your forked repository locally `git clone git@github.com:kvz/fig.git`.
1. Enter the local directory `cd fig`.
1. Set up a development environment `python setup.py develop`. That will install the dependencies and set up a symlink from your `fig` executable to the checkout of the repo. So from any of your fig projects, `fig` now refers to your development project. Time to start hacking : )
1. Works for you? Run the test suite via `./scripts/test` to verify it won't break other usecases.
1. All good? Commit and push to GitHub, and submit a pull request.
