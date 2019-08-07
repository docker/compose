# Release HOWTO

This file describes the process of making a public release of `docker-compose`.
Please read it carefully before proceeding!

## Prerequisites

The following things are required to bring a release to a successful conclusion

### Local Docker engine (Linux Containers)

The release script builds images that will be part of the release.

### Docker Hub account

You should be logged into a Docker Hub account that allows pushing to the
following repositories:

- docker/compose
- docker/compose-tests

### Python

The release script is written in Python and requires Python 3.3 at minimum.

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

### A Bintray account and Bintray API key

Your Bintray account will need to be an admin member of the
[docker-compose organization](https://bintray.com/docker-compose).
Additionally, you should generate a personal API key. To do so, click your
username in the top-right hand corner and select "Edit profile" ; on the new
page, select "API key" in the left-side menu.

This API key should be exposed to the release script through the
`BINTRAY_TOKEN` environment variable.

### A PyPi account

Said account needs to be a member of the maintainers group for the
[`docker-compose` project](https://pypi.org/project/docker-compose/).

Moreover, the `~/.pypirc` file should exist on your host and contain the
relevant pypi credentials.

The following is a sample `.pypirc` provided as a guideline:

```
[distutils]
index-servers =
    pypi

[pypi]
username = user
password = pass
```

## Start a feature release

A feature release is a release that includes all changes present in the
`master` branch when initiated. It's typically versioned `X.Y.0-rc1`, where
Y is the minor version of the previous release incremented by one. A series
of one or more Release Candidates (RCs) should be made available to the public
to find and squash potential bugs.

From the root of the Compose repository, run the following command:
```
./script/release/release.sh -b <BINTRAY_USERNAME> start X.Y.0-rc1
```

After a short initialization period, the script will invite you to edit the
`CHANGELOG.md` file. Do so by being careful to respect the same format as
previous releases. Once done, the script will display a `diff` of the staged
changes for the bump commit. Once you validate these, a bump commit will be
created on the newly created release branch and pushed remotely.

The release tool then waits for the CI to conclude before proceeding.
If failures are reported, the release will be aborted until these are fixed.
Please refer to the "Resume a draft release" section below for more details.

Once all resources have been prepared, the release script will exit with a
message resembling this one:

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
./script/release/release.sh -b <BINTRAY_USERNAME> start --patch=BASE_VERSION RELEASE_VERSION
```

The process of starting a patch release is identical to starting a feature
release except for one difference ; at the beginning, the script will ask for
PR numbers you wish to cherry-pick into the release. These numbers should
correspond to existing PRs on the docker/compose repository. Multiple numbers
should be separated by whitespace.

Once you are ready to finalize the release (making binaries and other versioned
assets public), proceed to the "Finalize a release" section of this guide.

## Finalize a release

Once you're ready to make your release public, you may execute the following
command from the root of the Compose repository:
```
./script/release/release.sh -b <BINTRAY_USERNAME> finalize RELEASE_VERSION
```

Note that this command will create and publish versioned assets to the public.
As a result, it can not be reverted. The command will perform some basic
sanity checks before doing so, but it is your responsibility to ensure
everything is in order before pushing the button.

After the command exits, you should make sure:

- The `docker/compose:VERSION` image is available on Docker Hub and functional
- The `pip install -U docker-compose==VERSION` command correctly installs the
  specified version
- The install command on the Github release page installs the new release

## Resume a draft release

"Resuming" a release lets you address the following situations occurring before
a release is made final:

- Cherry-pick additional PRs to include in the release
- Resume a release that was aborted because of CI failures after they've been
  addressed
- Rebuild / redownload assets after manual changes have been made to the
  release branch
- etc.

From the root of the Compose repository, run the following command:
```
./script/release/release.sh -b <BINTRAY_USERNAME> resume RELEASE_VERSION
```

The release tool will attempt to determine what steps it's already been through
for the specified release and pick up where it left off. Some steps are
executed again no matter what as it's assumed they'll produce different
results, like building images or downloading binaries.

## Cancel a draft release

If issues snuck into your release branch, it is sometimes easier to start from
scratch. Before a release has been finalized, it is possible to cancel it using
the following command:
```
./script/release/release.sh -b <BINTRAY_USERNAME> cancel RELEASE_VERSION
```

This will remove the release branch with this release (locally and remotely),
close the associated PR, remove the release page draft on Github and delete
the Bintray repository for it, allowing you to start fresh.

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
`./script/release/release.sh --help`.
