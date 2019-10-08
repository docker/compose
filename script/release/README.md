# Release HOWTO

This file describes the process of making a public release of `docker-compose`.
Please read it carefully before proceeding!

## Prerequisites

The following things are required to bring a release to a successful conclusion

### A Github account and Github API token

Your Github account needs to have write access on the `docker/compose` repo.
To generate a Github token, head over to the
[Personal access tokens](https://github.com/settings/tokens) page in your
Github settings and select "Generate new token". Your token should include
(at minimum) the following scopes:

- `repo:status`
- `public_repo`

This API token should be exposed to the release script through the
`GITHUB_TOKEN` environment variable.

## Start a feature release

A feature release is a release that includes all changes present in the
`master` branch when initiated. It's typically versioned `X.Y.0-rc1`, where
Y is the minor version of the previous release incremented by one. A series
of one or more Release Candidates (RCs) should be made available to the public
to find and squash potential bugs.

From the root of the Compose repository, run the following command:
```
./script/release/pre-release.sh X.Y.0-rc1
```

After a short initialization period, the script will invite you to edit the
`CHANGELOG.md` file. Do so by being careful to respect the same format as
previous releases. Once done, the script will display a `diff` of the staged
changes for the bump commit. Once you validate these, a bump commit will be
created on the newly created release branch and pushed remotely.

```
You're almost done! Please verify that everything is in order and you are ready
to make the release public, then run the following command:
./script/release/release.sh -b user finalize X.Y.0-rc1
```

Once you are ready to finalize the release (making binaries and other versioned
assets public), proceed to the "Finalize a release" section of this guide.

## Start a patch release

A patch release is a release that builds off a previous release with discrete
additions. This can be an RC release after RC1 (`X.Y.0-rcZ`, `Z > 1`), a GA release
based off the final RC (`X.Y.0`), or a bugfix release based off a previous
GA release (`X.Y.Z`, `Z > 0`).

From the root of the Compose repository, run the following command:
```
./script/release/pre-release.sh start --patch=BASE_VERSION RELEASE_VERSION
```

The process of starting a patch release is identical to starting a feature
release except for one difference ; at the beginning, the script will ask for
PR numbers you wish to cherry-pick into the release. These numbers should
correspond to existing PRs on the docker/compose repository. Multiple numbers
should be separated by whitespace.

## Finalize a release

Once you're ready to make your release public, go to the compose CI tags page
and run the release build.

Note that this action will create and publish versioned assets to the public.
As a result, it can not be reverted. The release build will perform some basic
sanity checks before doing so, but it is your responsibility to ensure
everything is in order before pushing the button.

After the command exits, you should make sure:

- The `docker/compose:VERSION` image is available on Docker Hub and functional
- The `pip install -U docker-compose==VERSION` command correctly installs the
  specified version
- The install command on the Github release page installs the new release

## Cancel a draft release

If issues snuck into your release branch, it is sometimes easier to start from
scratch. Before a release has been finalized, it is possible to cancel by deleting
the `bump-X.Y.Z` branches on github and locally.

## Manual operations

Some common, release-related operations are not covered by this tool and should
be handled manually by the operator:

- After any release:
    - Announce new release on Slack
- After a GA release:
    - Close the release milestone
    - Merge back `CHANGELOG.md` changes from the `release` branch into `master`
    - Bump the version in `compose/__init__.py` to the *next* minor version
      number with `dev` appended. For example, if you just released `1.4.0`,
      update it to `1.5.0dev`
    - Update compose_version in [github.com/docker/docker.github.io/blob/master/_config.yml](https://github.com/docker/docker.github.io/blob/master/_config.yml) and [github.com/docker/docker.github.io/blob/master/_config_authoring.yml](https://github.com/docker/docker.github.io/blob/master/_config_authoring.yml)
    - Update the release note in [github.com/docker/docker.github.io](https://github.com/docker/docker.github.io/blob/master/release-notes/docker-compose.md)

## Advanced options

You can consult the full list of options for the release tool by executing
`./script/release/pre-release.sh --help`.
