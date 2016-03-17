# OCI Specs Roadmap

This document serves to provide a long term roadmap on our quest to a 1.0 version of the OCI container specification.
Its goal is to help both maintainers and contributors find meaningful tasks to focus on and create a low noise environment.
The items in the 1.0 roadmap can be broken down into smaller milestones that are easy to accomplish.
The topics below are broad and small working groups will be needed for each to define scope and requirements or if the feature is required at all for the OCI level.
Topics listed in the roadmap do not mean that they will be implemented or added but are areas that need discussion to see if they fit in to the goals of the OCI.

Listed topics may defer to the [project wiki](https://github.com/opencontainers/specs/wiki/RoadMap:) for collaboration.

## 1.0

### Digest and Hashing

A bundle is designed to be moved between hosts.
Although OCI doesn't define a transport method we should have a cryptographic digest of the on-disk bundle that can be used to verify that a bundle is not corrupted and in an expected configuration.

*Owner:* philips

### Define Container Lifecycle

Containers have a lifecycle and being able to identify and document the lifecycle of a container is very helpful for implementations of the spec.
The lifecycle events of a container also help identify areas to implement hooks that are portable across various implementations and platforms.

*Owner:* mrunalp

### Define Standard Container Actions (Target release: v0.3.0)

Define what type of actions a runtime can perform on a container without imposing hardships on authors of platforms that do not support advanced options.

*Owner:* duglin

### Container Definition

Define what a software container is and its attributes in a cross platform way.

Could be solved by lifecycle/ops and create/start split discussions

*Owner:* vishh & duglin

### Live Container Updates

Should we allow dynamic container updates to runtime options?

Proposal: make it an optional feature

*Owner:* hqhq (was vishh) robdolinms, bcorrie

### Validation Tooling (Target release: v0.3.0)

Provide validation tooling for compliance with OCI spec and runtime environment.

*Owner:* mrunalp

### Testing Framework

Provide a testing framework for compliance with OCI spec and runtime environment.

*Owner:* liangchenye

### Version Schema

Decide on a robust versioning schema for the spec as it evolves.

Resolved but release process could evolve. Resolved for v0.2.0, expect to revisit near v1.0.0

*Owner:* vbatts

### Printable/Compiled Spec

Regardless of how the spec is written, ensure that it is easy to read and follow for first time users.

Part of this is resolved.  Produces an html & pdf.
Done
Would be nice to publish to the OCI web site as part of our release process.

*Owner:* vbatts

### Base Config Compatibility

Ensure that the base configuration format is viable for various platforms.

Systems:

* Solaris
* Windows
* Linux

*Owner:* robdolinms as lead coordinator

### Full Lifecycle Hooks

Ensure that we have lifecycle hooks in the correct places with full coverage over the container lifecycle.

Will probably go away with Vish's work on splitting create and start, and if we have exec.

*Owner:*

### Distributable Format

A common format for serializing and distributing bundles.

*Owner:* vbatts
