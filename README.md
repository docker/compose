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

The new CLI delegates to the classic docker for default contexts ; delegation is done to `com.docker.cli`. 
* `make moby-cli-link` will create a `com.docker.cli` link in `/usr/local/bin` if you don't already have it from Docker Desktop

## Building the project

```bash
$ make
```

This will make the cli with all backends enabled. `make cross` on the other hand will cross-compile the cli without the
example and local backend. We use `make cross` to build for our release, hence the exclusion of those backends. You can
still cross-compile with all backends enabled: `BUILD_TAGS=example,local make cross`.

If you make changes to the `.proto` files, make sure to `make protos` to generate go code.

## Tests

### unit tests

```
make test
```

If you need to update a golden file simply do `go test ./... -test.update-golden`.

### e2e tests

```
make e2e_local
```
This requires a local docker engine running

```
AZURE_TENANT_ID="xxx" AZURE_CLIENT_ID="yyy" AZURE_CLIENT_SECRET="yyy" make e2e_aci
```

This requires azure service principal credentials to login to azure. 
To get the values to be set in local environment variables, you can create a new service principal once you're logged in azure (with `docker login azure`)    
```
az ad sp create-for-rbac --name 'MyTestServicePrincipal' --sdk-auth
```
Running aci e2e tests will override your local login, the service principal credentials use a token that cannot be refreshed automatically. 
You might need to run again `docker login azure` to properly use the command line after running ACI e2e tests.

You can also run a single ACI test from the test suite : 
```
TESTIFY=TestACIRunSingleContainer AZURE_TENANT_ID="xxx" AZURE_CLIENT_ID="yyy" AZURE_CLIENT_SECRET="yyy" make e2e-aci
```

## Release

To create a new release:   
* check that the CI is green on the master commit you want to release 
* simply create a new tag of th form vx.y.z, following existing tags, and push the tag

Pushing the tag will automatically ceate a new release and make binaries (mac, win, linux) available for download. 

Note: Linux binaries are not automatically copied to /docker/aci-integration-beta, if you want to make the linux binary publically available, you'll need to manually create a release in aci-integration-beta and upload the binary.  
For Desktop integration, you need to make a PR in /docker/pinata and update the cli release number [here](https://github.com/docker/pinata/blob/master/build.json#L25)
