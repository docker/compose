# Release HOWTO

The release process is fully automated by `Release.Jenkinsfile`.

## Usage

1. edit `compose/__init__.py` to set release version number
1. commit and tag as `v{major}.{minor}.{patch}`
1. edit `compose/__init__.py` again to set next development version number
