Building a Compose release
==========================

## Prerequisites

The release scripts require the following tools installed on the host:

* https://hub.github.com/
* https://stedolan.github.io/jq/
* http://pandoc.org/

## To get started with a new release

Create a branch, update version, and add release notes by running `make-branch`

        ./script/release/make-branch $VERSION [$BASE_VERSION]

`$BASE_VERSION` will default to master. Use the last version tag for a bug fix
release.

As part of this script you'll be asked to:

1.  Update the version in `compose/__init__.py` and `script/run/run.sh`.

    If the next release will be an RC, append `-rcN`, e.g. `1.4.0-rc1`.

2.  Write release notes in `CHANGELOG.md`.

    Almost every feature enhancement should be mentioned, with the most
    visible/exciting ones first. Use descriptive sentences and give context
    where appropriate.

    Bug fixes are worth mentioning if it's likely that they've affected lots
    of people, or if they were regressions in the previous version.

    Improvements to the code are not worth mentioning.

3.  Create a new repository on [bintray](https://bintray.com/docker-compose).
    The name has to match the name of the branch (e.g. `bump-1.9.0`) and the
    type should be "Generic". Other fields can be left blank.

4.  Check that the `vnext-compose` branch on
    [the docs repo](https://github.com/docker/docker.github.io/) has
    documentation for all the new additions in the upcoming release, and create
    a PR there for what needs to be amended.


## When a PR is merged into master that we want in the release

1. Check out the bump branch and run the cherry pick script

        git checkout bump-$VERSION
        ./script/release/cherry-pick-pr $PR_NUMBER

2. When you are done cherry-picking branches move the bump version commit to HEAD

        ./script/release/rebase-bump-commit
        git push --force $USERNAME bump-$VERSION


## To release a version (whether RC or stable)

Check out the bump branch and run the `build-binaries` script

        git checkout bump-$VERSION
        ./script/release/build-binaries

When prompted build the non-linux binaries and test them.

1.  Download the different platform binaries by running the following script:

    `./script/release/download-binaries $VERSION`

    The binaries for Linux, OSX and Windows will be downloaded in the `binaries-$VERSION` folder.

3.  Draft a release from the tag on GitHub (the `build-binaries` script will open the window for
    you)

    The tag will only be present on Github when you run the `push-release`
    script in step 7, but you can pre-fill it at that point.

4.  Paste in installation instructions and release notes. Here's an example -
    change the Compose version and Docker version as appropriate:

        If you're a Mac or Windows user, the best way to install Compose and keep it up-to-date is **[Docker for Mac and Windows](https://www.docker.com/products/docker)**.

        Docker for Mac and Windows will automatically install the latest version of Docker Engine for you.

        Alternatively, you can use the usual commands to install or upgrade Compose:

        ```
        curl -L https://github.com/docker/compose/releases/download/1.16.0/docker-compose-`uname -s`-`uname -m` > /usr/local/bin/docker-compose
        chmod +x /usr/local/bin/docker-compose
        ```

        See the [install docs](https://docs.docker.com/compose/install/) for more install options and instructions.

        ## Compose file format compatibility matrix

        | Compose file format | Docker Engine |
        | --- | --- |
        | 3.3 | 17.06.0+ |
        | 3.0 &ndash; 3.2 | 1.13.0+ |
        | 2.3| 17.06.0+ |
        | 2.2 | 1.13.0+ |
        | 2.1 | 1.12.0+ |
        | 2.0 | 1.10.0+ |
        | 1.0 | 1.9.1+ |

        ## Changes

        ...release notes go here...

5.  Attach the binaries and `script/run/run.sh`

6.  Add "Thanks" with a list of contributors. The contributor list can be generated
    by running `./script/release/contributors`.

7.  If everything looks good, it's time to push the release.


        ./script/release/push-release


8.  Merge the bump PR.

8.  Publish the release on GitHub.

9.  Check that all the binaries download (following the install instructions) and run.

10. Announce the release on the appropriate Slack channel(s).

## If it’s a stable release (not an RC)

1. Close the release’s milestone.

## If it’s a minor release (1.x.0), rather than a patch release (1.x.y)

1. Open a PR against `master` to:

    - update `CHANGELOG.md` to bring it in line with `release`
    - bump the version in `compose/__init__.py` to the *next* minor version number with `dev` appended. For example, if you just released `1.4.0`, update it to `1.5.0dev`.

2. Get the PR merged.

## Finally

1. Celebrate, however you’d like.
