# Building a Compose release

## Building binaries

`script/build-linux` builds the Linux binary inside a Docker container:

    $ script/build-linux

`script/build-osx` builds the Mac OS X binary inside a virtualenv:

    $ script/build-osx

For official releases, you should build inside a Mountain Lion VM for proper
compatibility. Run the this script first to prepare the environment before
building - it will use Homebrew to make sure Python is installed and
up-to-date.

    $ script/prepare-osx

## Release process

1. Open pull request that:
 - Updates the version in `compose/__init__.py`
 - Updates the binary URL in `docs/install.md`
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
