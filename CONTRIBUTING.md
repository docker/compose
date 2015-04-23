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
3. Enter the local directory `cd compose`.
4. Set up a development environment by running `python setup.py develop`. This
   will install the dependencies and set up a symlink from your `docker-compose`
   executable to the checkout of the repository. When you now run
   `docker-compose` from anywhere on your machine, it will run your development
   version of Compose.

## Running the test suite

Use the test script to run DCO check, linting checks and then the full test
suite against different Python interpreters:

    $ script/test

Tests are run against a Docker daemon inside a container, so that we can test
against multiple Docker versions. By default they'll run against only the latest
Docker version - set the `DOCKER_VERSIONS` environment variable to "all" to run
against all supported versions:

    $ DOCKER_VERSIONS=all script/test

Arguments to `script/test` are passed through to the `nosetests` executable, so
you can specify a test directory, file, module, class or method:

    $ script/test tests/unit
    $ script/test tests/unit/cli_test.py
    $ script/test tests.integration.service_test
    $ script/test tests.integration.service_test:ServiceTest.test_containers

Before pushing a commit you can check the DCO by invoking `script/validate-dco`.

## Building binaries

Linux:

    $ script/build-linux

OS X:

    $ script/build-osx

Note that this only works on Mountain Lion, not Mavericks, due to a
[bug in PyInstaller](http://www.pyinstaller.org/ticket/807).

## Release process

1. Open pull request that:
 - Updates the version in `compose/__init__.py`
 - Updates the binary URL in `docs/install.md`
 - Updates the script URL in `docs/completion.md`
 - Adds release notes to `CHANGES.md`
2. Create unpublished GitHub release with release notes
3. Build Linux version on any Docker host with `script/build-linux` and attach
   to release
4. Build OS X version on Mountain Lion with `script/build-osx` and attach to
   release as `docker-compose-Darwin-x86_64` and `docker-compose-Linux-x86_64`.
5. Publish GitHub release, creating tag
6. Update website with `script/deploy-docs`
7. Upload PyPi package

        $ git checkout $VERSION
        $ python setup.py sdist upload
