# containerkit

containerkit is a collection of components for building a fully featured container runtime, storage, and distribution layers in high level projects. 

## Scope and Principles

Having a clearly defined scope of a project is important for ensuring consistency and focus.
These following criteria will be used when reviewing pull requests, features, and changes for the project before being accepted.

### Components 

containerkit is a collection of components.
These components can be used independently or together.
They should not have tight dependencies on each other so that they are unable to be used independently but should be designed in a way that when used together the components have a natural flow to the APIs.

An example for this design can be seen with the overlay filesystems and the container execution layer.
The execution layer and overlay filesystems can be used independently but if you were to use both, they share a common `Mount` struct that the filesystems produce and the execution layer consumes to start a container inside a root filesystem.


### Primitives

containerkit should expose primitives to solve problems instead of building high level abstractions.
A common example of this is how build is implemented.
Instead of having a build API in containerkit we should expose the lower level primitives that allow things like build to work.
Breaking up the filesystem APIs to allow snapshots, copy functionality, and mounts allow people implementing build at the higher levels more flexibility.


### Extensibility and Defaults

For the various components in containerkit there should be defined extension points where implementations can be swapped for alternatives.
The best example of this is that containerkit will use `runc` from OCI as the default runtime in the execution layer but for other runtimes conforming to the OCI Runtime specification they can be easily added to contianerkit.

containerkit will come with a default implementation for the various components.
These defaults will be chosen my the maintainers of the project and should not change unless better tech for that component comes out.
Additional implementations will not be accepted into the core repository and should be developed in a separate repository not maintained by the containerkit maintainers.

### Scope

The following table specifies the various components of containerkit and general features of container runtimes.
The table specifies whether or not the feature/component is in or out of scope.

| Name           | Description                                                                                   | In/Out | Reason                                                                                                       |
|----------------|-----------------------------------------------------------------------------------------------|--------|--------------------------------------------------------------------------------------------------------------|
| execution      | Provide an extensible execution layer for executing a container                               | in     |                                                                                                              |
| cow filesystem | Built in functionality for overlay, aufs, and other copy on write filesystems for containers  | in     |                                                                                                              |
| distribution   | Having the ability to push, pull, package, and sign images                                    | in     |                                                                                                              |
| networking     | Providing network functionality to containers along with configuring their network namespaces | in     |                                                                                                              |
| build          | Building images as a first class API                                                          | out    | Build is a higher level tooling feature and can be implemented in many different ways on top of containerkit |
| volumes        | Provide primitives for volumes and persistent storage                                         |        |                                                                                                              |

containerkit is scoped to a single host.
It can be used to builds things like a node agent that launches containers but does not have any concepts of a distributed system.

Also things like service discovery are out of scope even though networking is in scope.
containerkit should provide the primitives to create, add, remove, or manage network interfaces for a container but ip allocation, discovery, and DNS should be handled at higher layers.

## Copyright and license

Copyright © 2016 Docker, Inc. All rights reserved, except as follows. Code
is released under the Apache 2.0 license. The README.md file, and files in the
"docs" folder are licensed under the Creative Commons Attribution 4.0
International License under the terms and conditions set forth in the file
"LICENSE.docs". You may obtain a duplicate copy of the same license, titled
CC-BY-SA-4.0, at http://creativecommons.org/licenses/by/4.0/.
