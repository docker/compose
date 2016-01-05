# Glossary

## Bundle

A [directory structure](bundle.md) that is written ahead of time, distributed, and used to seed the runtime for creating a [container](#container) and launching a process within it.

## Configuration

The [`config.json`](config.md) and [`runtime.json`](runtime-config.md) files in a [bundle](#bundle) which define the intended [container](#container) and container process.

## Container

An environment for executing processes with configurable isolation and resource limitations.
For example, namespaces, resource limits, and mounts are all part of the container environment.

## JSON

All configuration [JSON][] MUST be encoded in [UTF-8][].

## Runtime

An implementation of this specification.
It reads the [configuration files](#configuration) from a [bundle](#bundle), uses that information to create a [container](#container), launches a process inside the container, and performs other [lifecycle actions](runtime.md).

[JSON]: http://json.org/
[UTF-8]: http://www.unicode.org/versions/Unicode8.0.0/ch03.pdf
