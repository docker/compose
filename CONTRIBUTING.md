# Contributing to Fig

If you're looking contribute to [Fig](http://orchardup.github.io/fig/)
but you're new to the project or maybe even to Python, here are the steps
that should get you started.

1. Fork [https://github.com/orchardup/fig](https://github.com/orchardup/fig) to your username. kvz in this example.
1. Clone your forked repository locally `git clone git@github.com:kvz/fig.git`.
1. Enter the local directory `cd fig`.
1. Set up a development environment `python setup.py develop`. That will install the dependencies and set up a symlink from your `fig` executable to the checkout of the repo. So from any of your fig projects, `fig` now refers to your development project. Time to start hacking : )
1. Works for you? Run the test suite via `./scripts/test` to verify it won't break other usecases.
1. All good? Commit and push to GitHub, and submit a pull request.

## Running the test suite

    $ script/test

## Building binaries

Linux:

    $ script/build-linux

OS X:

    $ script/build-osx

Note that this only works on Mountain Lion, not Mavericks, due to a [bug in PyInstaller](http://www.pyinstaller.org/ticket/807).


