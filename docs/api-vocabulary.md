# Vocabulary of the `pkg/api` package

The options and results of `pkg/api` use a small set of terms. This page says what each one means, and where the
same word is used for more than one thing. When a field's documentation and this page disagree, the field wins:
please open an issue.

## Things Compose manages

- **Project**: the application, identified by its name. The containers, networks and volumes of a project carry the
  `com.docker.compose.project` label. `docker compose ls` lists projects (the `Stack` type of the API is a project,
  under the name `docker stack ls` gives it).
- **Service**: one entry of the `services` section of the Compose file. Its containers carry the
  `com.docker.compose.service` label.
- **Replica**: one of the containers of a service. Replicas are numbered from 1 (`com.docker.compose.container-number`);
  see [Index](#index).
- **One-off container**: a container created by `compose run`, labelled `com.docker.compose.oneoff=True`. It is not a
  replica of the service, and most operations leave it aside unless they say otherwise.
- **Orphan**: a container that belongs to the project but should not exist any more: either its service is not
  defined by the Compose model any more (neither enabled nor disabled by a profile), or it is a one-off container that has finished its task. A still-running
  one-off container is never an orphan.
- **Stopped container**: a container that is not running, for instance one that was created but never started, or
  that exited.

## Selecting what an operation acts on

### `Services`

`Services` is a list of service names. It is a filter nearly everywhere: the operation acts on those services, or on
all of them when the list is empty. There are three exceptions, where it does not filter:

| Where | What it does |
|---|---|
| `ProjectLoadOptions.Services` | narrows the loaded project to these services and their dependencies. This is where the scope of an operation is decided |
| `CreateOptions.Services`, `ScaleOptions.Services` | names the services the user targeted, which follow the `Recreate` policy while the others follow `RecreateDependencies`. The whole project still converges |
| `StartOptions.Services` | only scopes the log monitor of the interactive session of `Up` |

To scope an operation that takes a `*types.Project`, narrow the project itself before calling it.

### `Project`

Several options carry an optional `Project *types.Project`. For `Start`, `Stop`, `Restart`, `Kill` and `Down`, nil
means the project is rebuilt from the labels of its containers. `Pause` and `Remove` only use it to narrow the
services, `Logs` also uses it to scope the follow monitor, and `Ps` and `Attach` ignore it.

### Index

`Index` selects a replica by its number, starting at 1. `0` means no particular replica, and what that gives depends
on the operation: see the package documentation.

### The `Include…` switches

A boolean named `IncludeX` adds something the operation leaves out by default, and its zero value is what the
matching CLI command does without flags.

| Field | Adds |
|---|---|
| `ListOptions.IncludeStopped` | the stopped containers, so that a project whose containers are all stopped is listed |
| `PsOptions.IncludeStopped` | the stopped containers |
| `PsOptions.IncludeOneOff` | the one-off containers |
| `KillOptions.IncludeStopped` | the containers that are not running (the one-off ones are targeted either way) |
| `CopyOptions.IncludeOneOff` | the one-off containers of the service (when `Index` is 0) |
| `ProjectLoadOptions.IncludeUnused` | the resources no service uses |

The `All` field these replace has a different meaning in each struct. It still works, and is deprecated.

## Converging a project

- **Recreate policy** (`CreateOptions.Recreate`, `RecreateDependencies`): what to do with a container that already
  exists. `RecreateDiverged` (the default, also used for the empty string) recreates it when its configuration or
  its image changed, `RecreateForce` always recreates it, `RecreateNever` never does. Any other value is rejected by
  `CreateOptions.Validate`.
- **Anonymous volumes** of a recreated container are, by the CLI default, handed to the new container
  (`CreateOptions.Inherit` set to true). Beware that the zero value of `Inherit` does the opposite: it recreates them,
  like `up --renew-anon-volumes`.
- **Detached and interactive** are the two modes of `Up`, selected by `StartOptions.Attach`: nil returns once the
  containers are started, a consumer keeps a foreground session running (log streaming, cascade, menu).
  `UpOptions.Validate` lists the options that only make sense in one of them.

## Events

The word *event* is used for three different things:

| Type | What it is | Produced by |
|---|---|---|
| `Event` | a runtime event of a container (`start`, `die`…) | `Events` |
| `ContainerEvent` | what the log monitor reports about a container: a log line, a start, a stop, an exit | `Up`, `Logs`, `Attach` |
| `Resource` (with `EventStatus`) | the progress of an operation on a resource, delivered to an `EventProcessor` | every operation |

## Errors

The errors a program may need to tell apart match a sentinel with `errors.Is`: `ErrNotFound`, `ErrNoResources`,
`ErrNotRunning` (a service has no running container, `ServiceNotRunningError` carries the details),
`ErrIncompatibleOptions` and `ErrInvalidOption` (options rejected by a `Validate` method, before anything is done).

## Naming rules for new fields

- The zero value of an options struct is what the CLI does without flags. A boolean is therefore named after what
  it changes from that default, `IncludeStopped` and not `ExcludeStopped`.
- A word means one thing in the package. A new field must not reuse a word already taken in the sections above.
- A field with a closed set of values is typed, or at least documents its values and rejects the others in a
  `Validate` method.
