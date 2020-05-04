# Docker API

[![Actions Status](https://github.com/docker/api/workflows/Continuous%20integration/badge.svg)](https://github.com/docker/api/actions)

## Dev Setup

The recommended way is to use the main `Makefile` that runs everything inside a container.

If you don't have or want to use Docker for building you need to make sure you have all the needed tools installed locally:

* go 1.14
* `go get github.com/golang/protobuf/protoc-gen-go@v1.4.1`
* `go get golang.org/x/tools/cmd/goimports`
* `go get gotest.tools/gotestsum@v0.4.2`
* `go get github.com/golangci/golangci-lint/cmd/golangci-lint@v1.26.0`

And then you can call the same make targets but you need to pass it the `builder.Makefile` (`make -f builder.Makefile`).

## Building the project

```bash
$ make
```

If you make changes to the `.proto` files, make sure to `make protos` to generate go code.
