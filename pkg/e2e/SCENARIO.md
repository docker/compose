# E2E scenarios: the contract

With coding agents writing most of the production code, e2e tests are the
document humans actually read and review. A scenario must state an intent, a
compose model, and a sequence of steps `(command → expected observables)` —
and nothing else. Everything operational (project naming, cleanup, failure
diagnostics) belongs to the framework, not to the test.

The DSL lives in [`scenario.go`](scenario.go) (execution, actions,
requirements) and [`checks.go`](checks.go) (the vocabulary of observables).

## Writing a scenario

```go
func TestRestart(t *testing.T) {
	NewScenario(t, "restart must bring an exited service back up, restarting the same container").
		Compose(`
services:
  app:
    image: alpine
    init: true
    command: ash -c "if [[ -f /tmp/restart.lock ]] ; then sleep infinity; else touch /tmp/restart.lock; fi"
`).
		Step("up starts the service, whose first run exits at once",
			ComposeCmd("up", "-d"),
			Eventually(ServiceState("app", "exited"), 10*time.Second)).
		Step("restart brings the service back up, reusing the container",
			ComposeCmd("restart"),
			Eventually(ServiceState("app", "running"), 10*time.Second),
			NotRecreated("app"))
}
```

Rules:

- **New e2e tests use `NewScenario`.** The legacy `NewCLI` style remains for
  existing tests, converted opportunistically; don't add to it.
- **One intent = one invariant.** The intent is a one-line statement of the
  behavior being locked, phrased as an obligation ("X must Y"). If you need
  two intents, write two scenarios.
- **Step names are behavior sentences**, not command echoes: "an unchanged
  create is a no-op", not "run create again". The transcript of step names
  should read as the specification.
- **The compose model is inline.** A scenario is self-contained in the test
  source; no fixture directories. Interpolate runtime values via `Env`. When
  the project needs more than a `compose.yaml` (a Dockerfile, an env file, a
  config file), declare all files with `Files` as a [txtar](https://pkg.go.dev/golang.org/x/tools/txtar)
  archive — the format Go's own `cmd/go` tests are written in: diff-friendly,
  and one every human and coding agent already knows by heart:

  ```go
  s.Files(`
  -- compose.yaml --
  services:
    app:
      build: .
  -- Dockerfile --
  FROM alpine
  CMD ["sleep", "infinity"]
  `)
  ```
- **Regression tests link the issue** in a comment above the test, with a
  sentence on the failure mode being locked.

## Checks: observe real state

Checks are the shared vocabulary between scenarios; their discipline is what
keeps the contract meaningful.

- **Prefer state-based checks** (`ServiceState`, `NotRecreated`, `LabelSet`,
  `RunsOnPlatform`, …): they observe containers, labels and image manifests —
  what the user actually gets — not what the CLI printed.
- **`OutputContains` is a last resort**, legitimate only when the CLI's
  reported decision is itself the observable (e.g. "Skipped" vs "Pulled").
- **Never poll by hand**: wrap a state check in `Eventually(check, timeout)`.
  No `time.Sleep` in scenarios.
- **A new check must be generic** — no test-specific logic — **and named
  after the observable it asserts**, not after the test that needed it. It
  goes in `checks.go`, where the whole vocabulary is reviewed as one file.
  Before adding one, verify the observable isn't already expressible.
- A check should also fail loudly on a broken precondition (e.g.
  `NotRecreated` errors if the service had no container before the step)
  rather than pass vacuously.

## When a scenario fails

The report opens with everything needed to diagnose without re-running:

- **`artifacts: <dir>`** — a stable per-project directory holding the
  untruncated material: `compose.yaml`, `failure.txt`, each step's full
  command and output (`step-NN-*.txt`), `containers.txt`, `events.txt`, full
  container logs (`logs-*.txt`) and the per-step state snapshots
  (`snapshots.json`). Read these before re-running anything.
- **`E2E_KEEP_FAILED=1`** — rerun with this set to skip teardown of failed
  scenarios: containers, volumes and networks stay alive for `docker
  inspect`/`exec`. Clean up afterwards with
  `docker compose --project-name <project> down -v --remove-orphans`.
- The inline report shows the transcript (every step, exit code, duration),
  the failing step's output, project containers, engine events since the
  scenario started, and container log tails — truncated for readability; the
  artifacts have the full versions.

Run a single scenario with:

```sh
go test -tags e2e ./pkg/e2e/ -run TestRestart -v
```
