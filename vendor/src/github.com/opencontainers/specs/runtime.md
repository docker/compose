# Runtime and Lifecycle

## Scope of a Container

Barring access control concerns, the entity using a runtime to create a container MUST be able to use the operations defined in this specification against that same container.
Whether other entities using the same, or other, instance of the runtime can see that container is out of scope of this specification.

## State

The state of a container MUST include, at least, the following propeties:

* **`ociVersion`**: (string) is the OCI specification version used when creating the container.
* **`id`**: (string) is the container's ID.
This MUST be unique across all containers on this host.
There is no requirement that it be unique across hosts.
The ID is provided in the state because hooks will be executed with the state as the payload.
This allows the hooks to perform cleanup and teardown logic after the runtime destroys its own state.
* **`pid`**: (int) is the ID of the main process within the container, as seen by the host.
* **`bundlePath`**: (string) is the absolute path to the container's bundle directory.
This is provided so that consumers can find the container's configuration and root filesystem on the host.

When serialized in JSON, the format MUST adhere to the following pattern:
```json
{
    "ociVersion": "0.2.0",
    "id": "oci-container1",
    "pid": 4422,
    "bundlePath": "/containers/redis"
}
```

See [Query State](#query-state) for information on retrieving the state of a container.

## Lifecycle
The lifecycle describes the timeline of events that happen from when a container is created to when it ceases to exist.

1. OCI compliant runtime is invoked with a reference to the location of the bundle.
   How this reference is passed to the runtime is an implementation detail.
2. The container's runtime environment MUST be created according to the configuration in [`config.json`](config.md).
   Any updates to `config.json` after container is running MUST not affect the container.
3. The prestart hooks MUST be invoked by the runtime.
   If any prestart hook fails, then the container MUST be stopped and the lifecycle continues at step 8.
4. The user specified process MUST be executed in the container.
5. The poststart hooks MUST be invoked by the runtime.
   If any poststart hook fails, then the container MUST be stopped and the lifecycle continues at step 8.
6. Additional actions such as pausing the container, resuming the container or signaling the container MAY be performed using the runtime interface.
   The container MAY also error out, exit or crash.
7. The container MUST be destroyed by undoing the steps performed during create phase (step 2).
8. The poststop hooks MUST be invoked by the runtime and errors, if any, MAY be logged.

Note: The lifecycle is a WIP and it will evolve as we have more use cases and more information on the viability of a separate create phase.

## Operations

OCI compliant runtimes MUST support the following operations, unless the operation is not supported by the base operating system.

### Errors
In cases where the specified operation generates an error, this specification does not mandate how, or even if, that error is returned or exposed to the user of an implementation.
Unless otherwise stated, generating an error MUST leave the state of the environment as if the operation were never attempted - modulo any possible trivial ancillary changes such as logging.

### Query State

`state <container-id>`

This operation MUST generate an error if it is not provided the ID of a container.
This operation MUST return the state of a container as specified in the [State](#state) section.
In particular, the state MUST be serialized as JSON.


### Start

`start <container-id> <path-to-bundle>`

This operation MUST generate an error if it is not provided a path to the bundle and the container ID to associate with the container.
If the ID provided is not unique across all containers within the scope of the runtime, or is not valid in any other way, the implementation MUST generate an error.
Using the data in `config.json`, that are in the bundle's directory, this operation MUST create a new container.
This includes creating the relevant namespaces, resource limits, etc and configuring the appropriate capabilities for the container.
A new process within the scope of the container MUST be created as specified by the `config.json` file otherwise an error MUST be generated.

Attempting to start an already running container MUST have no effect on the container and MUST generate an error.

### Stop

`stop <container-id>`

This operation MUST generate an error if it is not provided the container ID.
This operation MUST stop and delete a running container.
Stopping a container MUST stop all of the processes running within the scope of the container.
Deleting a container MUST delete the associated namespaces and resources associated with the container.
Once a container is deleted, its `id` MAY be used by subsequent containers.
Attempting to stop a container that is not running MUST have no effect on the container and MUST generate an error.

### Exec

`exec <container-id> <path-to-json>`

This operation MUST generate an error if it is not provided the container ID and a path to the JSON describing the process to start.
The JSON describing the new process MUST adhere to the [Process configuration](config.md#process-configuration) definition.
This operation MUST create a new process within the scope of the container.
If the container is not running then this operation MUST have no effect on the container and MUST generate an error.
Executing this operation multiple times MUST result in a new process each time.
Example:
```
{
    "terminal": true,
    "user": {
        "uid": 0,
        "gid": 0,
        "additionalGids": null
    },
    "args": [
        "/bin/sleep",
        "60"
    ],
    "env": [
        "version=1.0"
    ],
    "cwd": "...",
}
```
This specification does not manadate the name of this JSON file.
See the specification of the `config.json` file for the definition of these fields.
The stopping, or exiting, of these secondary process MUST have no effect on the state of the container.
In other words, a container (and its PID 1 process) MUST NOT be stopped due to the exiting of a secondary process.

## Hooks

Many of the operations specified in this specification have "hooks" that allow for additional actions to be taken before or after each operation.
See [runtime configuration for hooks](./config.md#hooks) for more information.
