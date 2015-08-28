Building a Compose release
==========================

## To get started with a new release

1.  Create a `bump-$VERSION` branch off master:

        git checkout -b bump-$VERSION master

2.  Merge in the `release` branch on the upstream repo, discarding its tree entirely:

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

1.  Check that CI is passing on the bump PR.

2.  Check out the bump branch:

        git checkout bump-$VERSION

3.  Build the Linux binary:

        script/build-linux

4.  Build the Mac binary in a Mountain Lion VM:

        script/prepare-osx
        script/build-osx

5.  Test the binaries and/or get some other people to test them.

6.  Create a tag:

        TAG=$VERSION # or $VERSION-rcN, if it's an RC
        git tag $TAG

7.  Push the tag to the upstream repo:

        git push git@github.com:docker/compose.git $TAG

8.  Draft a release from the tag on GitHub.

    - Go to https://github.com/docker/compose/releases and click "Draft a new release".
    - In the "Tag version" dropdown, select the tag you just pushed.

9.  Paste in installation instructions and release notes. Here's an example - change the Compose version and Docker version as appropriate:

        Firstly, note that Compose 1.5.0 requires Docker 1.8.0 or later.

        Secondly, if you're a Mac user, the **[Docker Toolbox](https://www.docker.com/toolbox)** will install Compose 1.5.0 for you, alongside the latest versions of the Docker Engine, Machine and Kitematic.

        Otherwise, you can use the usual commands to install/upgrade. Either download the binary:

            curl -L https://github.com/docker/compose/releases/download/1.5.0/docker-compose-`uname -s`-`uname -m` > /usr/local/bin/docker-compose
            chmod +x /usr/local/bin/docker-compose

        Or install the PyPi package:

            pip install -U docker-compose==1.5.0

        Here's what's new:

        ...release notes go here...

10.  Attach the binaries.

11. Don’t publish it just yet!

12. Upload the latest version to PyPi:

        python setup.py sdist upload

13. Check that the pip package installs and runs (best done in a virtualenv):

        pip install -U docker-compose==$TAG
        docker-compose version

14. Publish the release on GitHub.

15. Check that both binaries download (following the install instructions) and run.

16. Email maintainers@dockerproject.org and engineering@docker.com about the new release.

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
