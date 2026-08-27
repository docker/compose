# Environment variables Compose reads

Single registry of every environment variable the `docker compose` binary
itself recognizes: where it is read, at which moment, and its default. The
user-facing subset is documented on docs.docker.com; this file is the
exhaustive, code-accurate map, kept next to the code so a change to a read
site updates it in the same commit.

Variables consumed by services (interpolation in the compose file, `env_file`,
`environment:`) are out of scope — they belong to the model, not the binary.
So are the variables the docker CLI itself handles (`DOCKER_HOST`,
`DOCKER_CONTEXT`, `DOCKER_CONFIG`, TLS settings, …): compose inherits them
through `command.Cli` without reading them.

## When a variable is read — the three moments that decide what can set it

1. **Flag-default time** — evaluated while the cobra command tree is built,
   before any file is read. Only the actual process environment works; the
   project's `.env` can never influence these.
2. **After `.env` injection** — `setEnvWithDotEnv`
   (`cmd/compose/compose.go`) runs early in `PersistentPreRunE` and exports
   into the process environment every `COMPOSE_*` key found in the project's
   env files that is not already set in the process environment. Variables
   read after that point (still via `os.Getenv`/`os.LookupEnv`) can therefore
   come from the shell **or** from the project `.env` — shell wins. Caveat:
   the injection is skipped entirely for remote (OCI/Git) configs.
3. **Project environment** — read from `project.Environment` (the merged
   os-env ∪ env-files mapping compose-go builds, os wins) after the project
   is loaded. Same effective precedence as (2), but per-command: only
   commands that load a project get a value.

A variable read at flag-default time and never re-read cannot be set from the
project `.env` — that asymmetry is a recurring source of bug reports; when
touching a read site, prefer moments (2) or (3).

## Project selection and loading

| Variable | Equivalent flag | Read | Default / values |
|---|---|---|---|
| `COMPOSE_FILE` | `-f` | compose-go (`cli.WithConfigFileEnv`), moment 3-ish: honored from shell and project `.env` | default file discovery (`compose.yaml`…) |
| `COMPOSE_PATH_SEPARATOR` | — | compose-go, splits `COMPOSE_FILE` | `:` (unix), `;` (windows) |
| `COMPOSE_PROJECT_NAME` | `-p` | `os.Getenv` in `projectOrName`/`toProjectName` (`cmd/compose/compose.go`), moment 2 | derived from project directory name |
| `COMPOSE_PROFILES` | `--profile` | compose-go | none |
| `COMPOSE_ENV_FILES` | `--env-file` | flag default (moment 1 — cannot come from `.env`, by construction) | `.env` in project dir |
| `COMPOSE_DISABLE_ENV_FILE` | — | compose-go | `false` |
| `COMPOSE_CONVERT_WINDOWS_PATHS` | — | compose-go (volume paths in the model) | `false` |
| `COMPOSE_COMPATIBILITY` | `--compatibility` | project environment (moment 3, `cmd/compose/compose.go` + `pkg/compose/loader.go`) | `false` |

## Runtime behavior

| Variable | Equivalent flag | Read | Default / values |
|---|---|---|---|
| `COMPOSE_PARALLEL_LIMIT` | `--parallel` | `resolveMaxConcurrency`, moment 2; flag wins | `-1` (unlimited) |
| `COMPOSE_ANSI` | `--ansi` | `resolveAnsiMode`, moment 2; flag wins. Also checked directly in `cmd/compose/hooks.go` for OSC8 hyperlinks | `auto` / `never` / `always` |
| `COMPOSE_PROGRESS` | `--progress` | flag default (moment 1 — cannot come from `.env`) | `auto` / `tty` / `plain` / `json` / `quiet` |
| `COMPOSE_STATUS_STDOUT` | — | package `init()` (moment 1) | `false`; `true` routes status/progress output to stdout instead of stderr |
| `COMPOSE_MENU` | `--menu` | `up` only, moment 2; flag wins | `true` when attached to a TTY |
| `COMPOSE_IGNORE_ORPHANS` | — | project environment (moment 3) — by `up` and `run` only; `create` shares the options struct but reads neither orphan variable | `false` |
| `COMPOSE_REMOVE_ORPHANS` | `--remove-orphans` | inconsistent (see #14139): `up` reads it at moment 2, `down`/`kill` at moment 1 (so the project `.env` works for `up` but not for `down`/`kill`), `run` never reads it | `false` |
| `DOCKER_DEFAULT_PLATFORM` | — | project environment (moment 3), resolved into `service.Platform` by `applyPlatforms` (`cmd/compose/options.go`) for every container-creating command — the value feeds the service config-hash, so commands MUST resolve it identically or containers spuriously recreate | none |

## Build

| Variable | Read | Default / values |
|---|---|---|
| `BUILDX_BUILDER` | classic (non-bake) build path picks the buildx builder by name (`cmd/compose/build.go`) | current buildx builder |
| `BUILDKIT_PROGRESS` | bake path, fallback progress mode when compose's own mode is `auto` (`pkg/compose/build_bake.go`) | — |

## Watch

| Variable | Read | Default / values |
|---|---|---|
| `COMPOSE_WATCH_WINDOWS_BUFFER_SIZE` | `pkg/watch/notify.go`, Windows only | `65536` |

## Remote configs

| Variable | Read | Default / values |
|---|---|---|
| `COMPOSE_EXPERIMENTAL_GIT_REMOTE` | `pkg/remote/git.go`, process env | `true` — despite the EXPERIMENTAL name (kept for backward compatibility) this is an opt-out: set a falsy value to disable |
| `COMPOSE_EXPERIMENTAL_OCI_REMOTE` | `pkg/remote/oci.go`, process env | `true` — same opt-out semantics as its git sibling |

## Observability

| Variable | Read | Default / values |
|---|---|---|
| `OTEL_EXPORTER_OTLP_ENDPOINT` (and standard `OTEL_*` family) | `internal/tracing`, standard OpenTelemetry SDK resolution; a Docker-context-declared OTLP endpoint is used as an additional exporter | disabled |
| `NO_COLOR` | `cmd/compose/compose.go`, `cmd/compose/hooks.go` (moment 2); any non-empty value | color enabled |

## Historical — recognized only to tell you they do nothing

`COMPOSE_EXPERIMENTAL` (unwired feature-flag mechanism) and `COMPOSE_BAKE`
(bake is simply the default build path) are gone: no code reads them, setting
them has no effect.

| Variable | Status |
|---|---|
| `COMPOSE_EXPERIMENTAL_WATCH_TAR` | read by `pkg/compose/watch.go` only to warn that a falsy value is ignored — the tar-based synchronization is the only implementation |
