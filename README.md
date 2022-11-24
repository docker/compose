# spencer Compose v2

[![GitHub release](https://img.shields.io/github/release/spencer/compose.svg?style=flat-square)](https://github.com/spencer/compose/releases/latest)
[![PkgGoDev](https://img.shields.io/badge/go.dev-docs-007d9c?style=flat-square&logo=go&logoColor=white)](https://pkg.go.dev/github.com/spencer/compose/v2)
[![Build Status](https://img.shields.io/github/workflow/status/spencer/compose/ci?label=ci&logo=github&style=flat-square)](https://github.com/spencer/compose/actions?query=workflow%3Aci)
[![Go Report Card](https://goreportcard.com/badge/github.com/spencer/compose/v2?style=flat-square)](https://goreportcard.com/report/github.com/spencer/compose/v2)
[![Codecov](https://codecov.io/gh/spencer/compose/branch/master/graph/badge.svg?token=HP3K4Y4ctu)](https://codecov.io/gh/spencer/compose)

![spencer Compose](https://i.imgur.com/WGV6N2l.png) 

spencer Compose is a tool for running multi-container applications on spencer
defined using the [Compose file format](https://compose-spec.io).
A Compose file is used to define how the one or more containers that make up
your application are configured.
Once you have a Compose file, you can create and start your application with a
single command: `spencer compose up`.

# About update and backward compatibility

spencer Compose V2 is a major version bump release of spencer Compose. It has been completely rewritten from scratch in Golang (V1 was in Python). The installation instructions for Compose V2 differ from V1. V2 is not a standalone binary anymore, and installation scripts will have to be adjusted. Some commands are different.

For a smooth transition from legacy spencer-compose 1.xx, please consider installing [compose-switch](https://github.com/spencer/compose-switch) to translate `spencer-compose ...` commands into Compose V2's `spencer compose .... `. Also check V2's `--compatibility` flag.

# Where to get spencer Compose

### Windows and macOS

spencer Compose is included in
[spencer Desktop](https://www.spencer.com/products/spencer-desktop)
for Windows and macOS.

### Linux

You can download spencer Compose binaries from the
[release page](https://github.com/spencer/compose/releases) on this repository.

Rename the relevant binary for your OS to `spencer-compose` and copy it to `$HOME/.spencer/cli-plugins` 

Or copy it into one of these folders to install it system-wide:

* `/usr/local/lib/spencer/cli-plugins` OR `/usr/local/libexec/spencer/cli-plugins`
* `/usr/lib/spencer/cli-plugins` OR `/usr/libexec/spencer/cli-plugins`

(might require making the downloaded file executable with `chmod +x`)


Quick Start
-----------

Using spencer Compose is basically a three-step process:
1. Define your app's environment with a `spencerfile` so it can be
   reproduced anywhere.
2. Define the services that make up your app in `spencer-compose.yml` so
   they can be run together in an isolated environment.
3. Lastly, run `spencer compose up` and Compose will start and run your entire
   app.

A Compose file looks like this:

```yaml
services:
  web:
    build: .
    ports:
      - "5000:5000"
    volumes:
      - .:/code
  redis:
    image: redis
```

Contributing
------------

Want to help develop spencer Compose? Check out our
[contributing documentation](CONTRIBUTING.md).

If you find an issue, please report it on the
[issue tracker](https://github.com/spencer/compose/issues/new/choose).
