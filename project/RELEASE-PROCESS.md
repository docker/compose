Building a Compose release
==========================

## To get started with a new release

Create a branch, update version, and add release notes by running `make-branch`

        git checkout -b bump-$VERSION $BASE_VERSION

`$BASE_VERSION` will default to master. Use the last version tag for a bug fix
release.

        git fetch origin
        git merge --strategy=ours origin/release

3.  Update the version in `docs/install.md` and `compose/__init__.py`.

    If the next release will be an RC, append `rcN`, e.g. `1.4.0rc1`.

4.  Write release notes in `CHANGES.md`.

    Almost every feature enhancement should be mentioned, with the most visible/exciting ones first. Use descriptive sentences and give context where appropriate.

    Bug fixes are worth mentioning if it's likely that they've affected lots of people, or if they were regressions in the previous version.

    Improvements to the code are not worth mentioning.

5.   Add a bump commit:

        git commit -am "Bump $VERSION"

6.   Push the bump branch to your fork:

        git push --set-upstream $USERNAME bump-$VERSION

7.  Open a PR from the bump branch against the `release` branch on the upstream repo, **not** against master.

## When a PR is merged into master that we want in the release

1.  Check out the bump branch:

        git checkout bump-$VERSION

2.   Cherry-pick the merge commit, fixing any conflicts if necessary:

        git cherry-pick -xm1 $MERGE_COMMIT_HASH

3.  Add a signoff (it’s missing from merge commits):

        git commit --amend --signoff

4.  Move the bump commit back to the tip of the branch:

        git rebase --interactive $PARENT_OF_BUMP_COMMIT

5.  Force-push the bump branch to your fork:

        git push --force $USERNAME bump-$VERSION

## To release a version (whether RC or stable)

Check out the bump branch and run the `push-release` script

        git checkout bump-$VERSION
        ./script/release/push-release $VERSION


When prompted test the binaries.


1.  Draft a release from the tag on GitHub (the script will open the window for
    you)

    In the "Tag version" dropdown, select the tag you just pushed.

2.  Paste in installation instructions and release notes. Here's an example - change the Compose version and Docker version as appropriate:

        Firstly, note that Compose 1.5.0 requires Docker 1.8.0 or later.

        Secondly, if you're a Mac user, the **[Docker Toolbox](https://www.docker.com/toolbox)** will install Compose 1.5.0 for you, alongside the latest versions of the Docker Engine, Machine and Kitematic.

        Otherwise, you can use the usual commands to install/upgrade. Either download the binary:

            curl -L https://github.com/docker/compose/releases/download/1.5.0/docker-compose-`uname -s`-`uname -m` > /usr/local/bin/docker-compose
            chmod +x /usr/local/bin/docker-compose

        Or install the PyPi package:

            pip install -U docker-compose==1.5.0

        Here's what's new:

        ...release notes go here...

3.  Attach the binaries.

4.  Publish the release on GitHub.

5.  Check that both binaries download (following the install instructions) and run.

6.  Email maintainers@dockerproject.org and engineering@docker.com about the new release.

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
