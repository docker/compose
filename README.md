# Docker API

[![Actions Status](https://github.com/docker/api/workflows/Continuous%20integration/badge.svg)](https://github.com/docker/api/actions)

## Dev Setup

The recommended way is to use the main `Makefile` that runs everything inside a container.

If you don't have or want to use Docker for building you need to make sure you have all the needed tools installed locally:

* go 1.14
* [protoc](https://github.com/protocolbuffers/protobuf)
* `go get github.com/golang/protobuf/protoc-gen-go@v1.4.1`
* `go get golang.org/x/tools/cmd/goimports`
* `go get github.com/golangci/golangci-lint/cmd/golangci-lint@v1.26.0`

And then you can call the same make targets but you need to pass it the `builder.Makefile` (`make -f builder.Makefile`).

The new CLI delegates to the classic docker for default contexts ; delegation is done to `docker-classic`. 
* `make classic-link` will create a `docker-classic` link in `/usr/local/bin` if you don't already have it from Docker Desktop

## Building the project

```bash
$ make
```

This will make the cli with all backends enabled. `make cross` on the other hand will cross-compile the cli without the
example and local backend. We use `make cross` to build for our release, hence the exclusion of those backends. You can
still cross-compile with all backends enabled: `BUILD_TAGS=example,local make cross`.

If you make changes to the `.proto` files, make sure to `make protos` to generate go code.

## Tests

To run unit tests:

```
make test
```

If you need to update a golden file simply do `go test ./... -test.update-golden`.
