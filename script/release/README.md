# Release HOWTO

The release process is fully automated by `Release.Jenkinsfile`.

## Usage

1. In the appropriate branch, run `./scripts/release/release tag <version>`

By appropriate, we mean for a version `1.26.0` or `1.26.0-rc1` you should run the script in the `1.26.x` branch.

The script should check the above then ask for changelog modifications.

After the executions, you should have a commit with the proper bumps for `docker-compose version` and `run.sh`

2. Run `git push --tags upstream <version_branch>`
This should trigger a new CI build on the new tag. When the CI finishes with the tests and builds a new draft release would be available on github's releases page.

3. Check and confirm the release on github's release page.
