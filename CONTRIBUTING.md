# Contributing to Compose

Compose is a part of the Docker project, and follows the same rules and principles. Take a read of [Docker's contributing guidelines](https://github.com/docker/docker/blob/master/CONTRIBUTING.md) to get an overview.

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

1. Fork [https://github.com/docker/compose](https://github.com/docker/compose) to your username.
1. Clone your forked repository locally `git clone git@github.com:yourusername/compose.git`.
1. Enter the local directory `cd compose`.
1. Set up a development environment by running `python setup.py develop`. This will install the dependencies and set up a symlink from your `docker-compose` executable to the checkout of the repository. When you now run `docker-compose` from anywhere on your machine, it will run your development version of Compose.

## Running the test suite

    $ script/test

## Building binaries

Linux:

    $ script/build-linux

OS X:

    $ script/build-osx

Note that this only works on Mountain Lion, not Mavericks, due to a [bug in PyInstaller](http://www.pyinstaller.org/ticket/807).

## Release process

1. Open pull request that:

 - Updates version in `compose/__init__.py`
 - Updates version in `docs/install.md`
 - Adds release notes to `CHANGES.md`

2. Create unpublished GitHub release with release notes

3. Build Linux version on any Docker host with `script/build-linux` and attach to release

4. Build OS X version on Mountain Lion with `script/build-osx` and attach to release as `docker-compose-Darwin-x86_64` and `docker-compose-Linux-x86_64`.

5. Publish GitHub release, creating tag

6. Update website with `script/deploy-docs`

7. Upload PyPi package

        $ git checkout $VERSION
        $ python setup.py sdist upload
