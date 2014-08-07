# Contributing to Fig

## Development environment

If you're looking contribute to [Fig](http://www.fig.sh/)
but you're new to the project or maybe even to Python, here are the steps
that should get you started.

1. Fork [https://github.com/docker/fig](https://github.com/docker/fig) to your username. kvz in this example.
1. Clone your forked repository locally `git clone git@github.com:kvz/fig.git`.
1. Enter the local directory `cd fig`.
1. Set up a development environment `python setup.py develop`. That will install the dependencies and set up a symlink from your `fig` executable to the checkout of the repo. So from any of your fig projects, `fig` now refers to your development project. Time to start hacking : )
1. Works for you? Run the test suite via `./script/test` to verify it won't break other usecases.
1. All good? Commit and push to GitHub, and submit a pull request.

## Running the test suite

    $ script/test

## Building binaries

Linux:

    $ script/build-linux

OS X:

    $ script/build-osx

Note that this only works on Mountain Lion, not Mavericks, due to a [bug in PyInstaller](http://www.pyinstaller.org/ticket/807).

## Sign your work

The sign-off is a simple line at the end of the explanation for the
patch, which certifies that you wrote it or otherwise have the right to
pass it on as an open-source patch.  The rules are pretty simple: if you
can certify the below (from [developercertificate.org](http://developercertificate.org/)):

    Developer's Certificate of Origin 1.1

    By making a contribution to this project, I certify that:

    (a) The contribution was created in whole or in part by me and I
        have the right to submit it under the open source license
        indicated in the file; or

    (b) The contribution is based upon previous work that, to the best
        of my knowledge, is covered under an appropriate open source
        license and I have the right under that license to submit that
        work with modifications, whether created in whole or in part
        by me, under the same open source license (unless I am
        permitted to submit under a different license), as indicated
        in the file; or

    (c) The contribution was provided directly to me by some other
        person who certified (a), (b) or (c) and I have not modified
        it.

    (d) I understand and agree that this project and the contribution
        are public and that a record of the contribution (including all
        personal information I submit with it, including my sign-off) is
        maintained indefinitely and may be redistributed consistent with
        this project or the open source license(s) involved.

then you just add a line saying

    Signed-off-by: Random J Developer <random@developer.example.org>

using your real name (sorry, no pseudonyms or anonymous contributions.)

The easiest way to do this is to use the `--signoff` flag when committing. E.g.:


    $ git commit --signoff


## Release process

1. Open pull request that:

 - Updates version in `fig/__init__.py`
 - Updates version in `docs/install.md`
 - Adds release notes to `CHANGES.md`

2. Create unpublished GitHub release with release notes

3. Build Linux version on any Docker host with `script/build-linux` and attach to release

4. Build OS X version on Mountain Lion with `script/build-osx` and attach to release

5. Publish GitHub release, creating tag

6. Update website with `script/deploy-docs`
