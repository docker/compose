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

1.  Update the version in `docs/install.md` and `compose/__init__.py`.

    If the next release will be an RC, append `rcN`, e.g. `1.4.0rc1`.

2.  Write release notes in `CHANGES.md`.

    Almost every feature enhancement should be mentioned, with the most visible/exciting ones first. Use descriptive sentences and give context where appropriate.

    Bug fixes are worth mentioning if it's likely that they've affected lots of people, or if they were regressions in the previous version.

    Improvements to the code are not worth mentioning.


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

1.  Download the osx binary from Bintray. Make sure that the latest build has
    finished, otherwise you'll be downloading an old binary.

    https://dl.bintray.com/docker-compose/$BRANCH_NAME/

2.  Download the windows binary from AppVeyor

    https://ci.appveyor.com/project/docker/compose

3.  Draft a release from the tag on GitHub (the script will open the window for
    you)

    In the "Tag version" dropdown, select the tag you just pushed.

4.  Paste in installation instructions and release notes. Here's an example - change the Compose version and Docker version as appropriate:

        Firstly, note that Compose 1.5.0 requires Docker 1.8.0 or later.

        Secondly, if you're a Mac user, the **[Docker Toolbox](https://www.docker.com/toolbox)** will install Compose 1.5.0 for you, alongside the latest versions of the Docker Engine, Machine and Kitematic.

        Otherwise, you can use the usual commands to install/upgrade. Either download the binary:

            curl -L https://github.com/docker/compose/releases/download/1.5.0/docker-compose-`uname -s`-`uname -m` > /usr/local/bin/docker-compose
            chmod +x /usr/local/bin/docker-compose

        Or install the PyPi package:

            pip install -U docker-compose==1.5.0

        Here's what's new:

        ...release notes go here...

5.  Attach the binaries and `script/run/run.sh`

6.  Add "Thanks" with a list of contributors. The contributor list can be generated
    by running `./script/release/contributors`.

7.  If everything looks good, it's time to push the release.


        ./script/release/push-release


8.  Publish the release on GitHub.

9.  Check that all the binaries download (following the install instructions) and run.

10. Email maintainers@dockerproject.org and engineering@docker.com about the new release.

## If it’s a stable release (not an RC)

1. Merge the bump PR.

2. Make sure `origin/release` is updated locally:

        git fetch origin

3. Update the `docs` branch on the upstream repo:

        git push git@github.com:docker/compose.git origin/release:docs

4. Let the docs team know that it’s been updated so they can publish it.

5. Close the release’s milestone.

## If it’s a minor release (1.x.0), rather than a patch release (1.x.y)

1. Open a PR against `master` to:

    - update `CHANGELOG.md` to bring it in line with `release`
    - bump the version in `compose/__init__.py` to the *next* minor version number with `dev` appended. For example, if you just released `1.4.0`, update it to `1.5.0dev`.

2. Get the PR merged.

## Finally

1. Celebrate, however you’d like.
