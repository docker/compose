# Project docs

## Release Process

* Increment version in version.go
* `git commit` version increment
* `git tag` the prior commit (preferrably signed tag)
* `make docs` to produce PDF and HTML copies of the spec
* Make a release on [github.com/opencontainers/specs](https://github.com/opencontainers/specs/releases) for the version. Attach the produced docs.

