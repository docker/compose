
### Prerequisites

* Windows:
  * [Docker Desktop](https://docs.docker.com/desktop/setup/install/windows-install/)
  * make
* macOS:
  * [Docker Desktop](https://docs.docker.com/desktop/setup/install/mac-install/)
  * make
* Linux:
  * [Docker 20.10 or later](https://docs.docker.com/engine/install/)
  * make

### Building the CLI

Once you have the prerequisites installed, you can build the CLI using:

```console
make
```

This will output a `docker-compose` CLI plugin for your host machine in
`./bin/build`.

You can statically cross compile the CLI for Windows, macOS, and Linux using the
`cross` target.

### Unit tests

To run all of the unit tests, run:

```console
make test
```

If you need to update a golden file simply do `go test ./... -test.update-golden`.

### End-to-end tests
To run e2e tests, the Compose CLI binary needs to be built. All the commands to run e2e tests propose a version
with the prefix `build-and-e2e` to first build the CLI before executing tests.

Note that this requires a local Docker Engine to be running.

#### Whole end-to-end tests suite

To execute both CLI and standalone e2e tests, run :

```console
make e2e
```

Or if you need to build the CLI, run:
```console
make build-and-e2e
```

#### Plugin end-to-end tests suite

To execute CLI plugin e2e tests, run :

```console
make e2e-compose
```

Or if you need to build the CLI, run:
```console
make build-and-e2e-compose
```

#### Standalone end-to-end tests suite

To execute the standalone CLI e2e tests, run :

```console
make e2e-compose-standalone
```

Or if you need to build the CLI, run:

```console
make build-and-e2e-compose-standalone
```

## Releases

To create a new release:
* Check that the CI is green on the main branch for the commit you want to release
* Run the release GitHub Actions workflow with a tag of form vx.y.z following existing tags.

This will automatically create a new tag, release and make binaries for
Windows, macOS, and Linux available for download on the
[releases page](https://github.com/docker/compose/releases).
