# Docker API

## Dev Setup

To setup a development machine to update the API protobufs, first run the `./setup-dev.sh` script
to install the correct version of protobufs on your system and get the protobuild binary.

## Building the API Project

```bash
> make
```

## Build the example backend

The example backend code is located in `/example/backend`.
Build the service with the resulting binary placed in the `/bin` directory.

```bash
> make example
```
