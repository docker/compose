
### Prerequisites

* Windows:
  * [Docker Desktop](https://hub.docker.com/editions/community/docker-ce-desktop-windows)
  * make
* macOS:
  * [Docker Desktop](https://hub.docker.com/editions/community/docker-ce-desktop-mac)
  * make
* Linux:
  * [Docker 19.03 or later](https://docs.docker.com/engine/install/)
  * make

### Building the CLI

Once you have the prerequisites installed, you can build the CLI using:

```console
make
```

This will output a `docker-compose` CLI plugin for your host machine in `./bin`.

You can statically cross compile the CLI for Windows, macOS, and Linux using the
`cross` target.

### Unit tests

To run all of the unit tests, run:

```console
make test
```

If you need to update a golden file simply do `go test ./... -test.update-golden`.

### End to end tests

To run the end to end tests, run:

```console
make e2e-compose
```

Note that this requires a local Docker Engine to be running.

## Releases

To create a new release:
* Check that the CI is green on the main branch for commit you want to release
* Run the release Github Actions workflow with a tag of the form vx.y.z following existing tags.

This will automatically create a new tag, release and make binaries for
Windows, macOS, and Linux available for download on the
[releases page](https://github.com/docker/compose/releases).
