# Contributing to Compose

Compose is a part of the Docker project, and follows the same rules and
principles. Take a read of [Docker's contributing guidelines](https://github.com/docker/docker/blob/master/CONTRIBUTING.md)
to get an overview.

## TL;DR

Pull requests will need:

 - Tests
 - Documentation
 - [To be signed off](https://github.com/docker/docker/blob/master/CONTRIBUTING.md#sign-your-work)
 - A logical series of [well written commits](https://github.com/alphagov/styleguides/blob/master/git.md)

## Development environment

If you're looking contribute to Compose
but you're new to the project or maybe even to Python, here are the steps
that should get you started.

1. Fork [https://github.com/docker/compose](https://github.com/docker/compose)
   to your username.
2. Clone your forked repository locally `git clone git@github.com:yourusername/compose.git`.
3. You must [configure a remote](https://help.github.com/articles/configuring-a-remote-for-a-fork/) for your fork so that you can [sync changes you make](https://help.github.com/articles/syncing-a-fork/) with the original repository.
4. Enter the local directory `cd compose`.
5. Make sure both [Docker](https://docs.docker.com/engine/installation/) and
   [dobi](https://dnephin.github.io/dobi/install.html) are installed.
6. Set up a development environment by running

       # Start a project shell in a container.
       $ dobi shell
       # Create the virtualenv for building and testing Compose
       $ tox --notest

## Install pre-commit hooks

This step is optional, but recommended. Pre-commit hooks will run style checks
and in some cases fix style issues for you, when you commit code.

Install the git pre-commit hooks using [tox](https://tox.readthedocs.io) by
running `tox -e pre-commit` or by following the
[pre-commit install guide](http://pre-commit.com/#install).

To run the style checks at any time run `dobi lint`.

## Submitting a pull request

See Docker's [basic contribution workflow](https://docs.docker.com/opensource/workflow/make-a-contribution/#the-basic-contribution-workflow) for a guide on how to submit a pull request for code or documentation.

## Running the test suite

Use the test script to run the full test suite:

    $ dobi test

Set the `DOCKER_VERSIONS` environment variable to "default" to run
against only the latest docker version, or set it to any version tag.

    $ DOCKER_VERSIONS=default dobi test

To run a subset of tests, enter into a project shell and run the test script
directly:

    $ dobi shell
    $ tox tests/unit
    $ tox tests/unit/cli_test.py
    $ tox tests/unit/config_test.py::ConfigTest
    $ tox tests/unit/config_test.py::ConfigTest::test_load

## Finding things to work on

We use a [ZenHub board](https://www.zenhub.io/) to keep track of specific things we are working on and planning to work on. If you're looking for things to work on, stuff in the backlog is a great place to start.

For more information about our project planning, take a look at our [GitHub wiki](https://github.com/docker/compose/wiki).
