# containerd

containerd is a daemon for managing images and containers on a single host.
It is built to be multi-tenant and handle multiple clients for container runtime needs.

## Features

* OCI Image Spec support
* OCI Runtime Spec support
* Image push and pull support
* Container runtime and lifecycle support
* Network primitives for creation, modification, and deletion of interfaces
* Multi-tenant supported with CAS storage for global images

## Scope and Principles

Having a clearly defined scope of a project is important for ensuring consistency and focus.
These following criteria will be used when reviewing pull requests, features, and changes for the project before being accepted.

### Components 

Components should not have tight dependencies on each other so that they are unable to be used independently.
The APIs for images and containers should be designed in a way that when used together the components have a natural flow but still be useful independently.

An example for this design can be seen with the overlay filesystems and the container execution layer.
The execution layer and overlay filesystems can be used independently but if you were to use both, they share a common `Mount` struct that the filesystems produce and the execution layer consumes.

### Primitives

containerd should expose primitives to solve problems instead of building high level abstractions in the API.
A common example of this is how build would be implemented.
Instead of having a build API in containerd we should expose the lower level primitives that allow things required in build to work.
Breaking up the filesystem APIs to allow snapshots, copy functionality, and mounts allow people implementing build at the higher levels more flexibility.

### Extensibility and Defaults

For the various components in containerd there should be defined extension points where implementations can be swapped for alternatives.
The best example of this is that containerd will use `runc` from OCI as the default runtime in the execution layer but other runtimes conforming to the OCI Runtime specification they can be easily added to containerd.

containerd will come with a default implementation for the various components.
These defaults will be chosen my the maintainers of the project and should not change unless better tech for that component comes out.
Additional implementations will not be accepted into the core repository and should be developed in a separate repository not maintained by the containerd maintainers.

### Releases 

Containerd will be released with a 1.0 when feature complete and this version will be supported for 1 year with security and bug fixes applied and released.

The upgrade path for containerd is that the 0.0.x patch relases are always backward compatible with its major and minor version.
Minor (0.x.0) version will always be compatible with the previous minor release. i.e. 1.2.0 is backwards compatible with 1.1.0 and 1.1.0 is compatible with 1.0.0.
There is no compatiability guarentes with upgrades from two minor relases.  i.e. 1.0.0 to 1.2.0.

There are not backwards compatability guarentes with upgrades to major versions.  i.e 1.0.0 to 2.0.0.
Each major version will be supported for 1 year with bug fixes and security patches.

### Scope

The following table specifies the various components of containerd and general features of container runtimes.
The table specifies whether or not the feature/component is in or out of scope.

| Name           | Description                                                                                   | In/Out | Reason                                                                                                       |
|----------------|-----------------------------------------------------------------------------------------------|--------|--------------------------------------------------------------------------------------------------------------|
| execution      | Provide an extensible execution layer for executing a container                               | in     |                                                                                                              |
| cow filesystem | Built in functionality for overlay, aufs, and other copy on write filesystems for containers  | in     |                                                                                                              |
| distribution   | Having the ability to push, pull, package, and sign images                                    | in     |                                                                                                              |
| networking     | Providing network functionality to containers along with configuring their network namespaces | in     |                                                                                                              |
| build          | Building images as a first class API                                                          | out    | Build is a higher level tooling feature and can be implemented in many different ways on top of containerd |
| volumes        | Provide primitives for volumes and persistent storage                                         |        |                                                                                                              |

containerd is scoped to a single host.
It can be used to builds things like a node agent that launches containers but does not have any concepts of a distributed system.

Also things like service discovery are out of scope even though networking is in scope.
containerd should provide the primitives to create, add, remove, or manage network interfaces for a container but ip allocation, discovery, and DNS should be handled at higher layers.

### How is the scope changed?

The scope of this project is a whitelist.
If its not mentioned as being in scope, it is out of scope.  
For the scope of this project to change it requires a 100% vote from all maintainers of the project.

## Copyright and license

Copyright © 2016 Docker, Inc. All rights reserved, except as follows. Code
is released under the Apache 2.0 license. The README.md file, and files in the
"docs" folder are licensed under the Creative Commons Attribution 4.0
International License under the terms and conditions set forth in the file
"LICENSE.docs". You may obtain a duplicate copy of the same license, titled
CC-BY-SA-4.0, at http://creativecommons.org/licenses/by/4.0/.
