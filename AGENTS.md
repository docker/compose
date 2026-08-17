# Project: Docker Compose

## Build & Test

- Build: `make build`
- Test all: `make test`
- Test unit: `go test ./pkg/...` — needs no Docker daemon; the e2e suite is
  gated behind the `e2e` build tag and is not picked up
- Test single: `go test ./pkg/compose/ -run TestFunctionName`
- E2E tests: `go test -tags e2e ./pkg/e2e/ -run TestName` — requires a Docker
  daemon and the locally built binary (`make build`)

## E2E tests

- **New e2e tests use the declarative `Scenario` DSL** (`NewScenario` in
  `pkg/e2e/scenario.go`): intent, inline compose model, steps as
  `(command → expected observables)`. Read `pkg/e2e/SCENARIO.md` before
  writing or debugging one — it codifies the rules (state-based checks first,
  `OutputContains` as last resort, new checks go in `pkg/e2e/checks.go`) and
  how to exploit failure artifacts and `E2E_KEEP_FAILED=1`.

## Architecture map

Read this before changing lifecycle or CLI code — several of these facts are
not guessable from local reading.

- **Packages**: `cmd/compose` is the cobra CLI layer; `pkg/api` is the public
  SDK contract (interface `Compose` + option structs); `pkg/compose` is the
  backend implementation (`NewComposeService`). Everything else under `pkg/`
  is exported for historical reasons, not as a stability promise.
- **Two lifecycle engines coexist.** The plan-based reconciler
  (`pkg/compose/reconcile.go`) has a single entry point: `create()` — so only
  `create`/`up`/`run`/`scale`/`watch` go through it. `start`, `stop`,
  `restart` and `down` use the imperative dependency-ordered engine
  (`pkg/compose/dependencies.go`) with the shared helpers in
  `service_containers.go`. The plan does **not** start containers: `up` runs
  a separate start phase (`start.go`) that re-lists containers and starts
  them in dependency order.
- **Operation convention**: exported `composeService` methods wrap the work
  in `Run(ctx, …, "opname")`, which drives the `EventProcessor`
  (`Start`/`On`/`Done`) used for progress display. Unexported methods
  (`s.create`, `s.start`, …) are building blocks without their own event
  cycle — calling an exported method from another operation nests event
  cycles.
- **Output conventions**: progress/status rendering goes to **stderr**
  (`dockerCli.Err()`); stdout is reserved for command payload (`ps`,
  `config`, container logs' stdout stream). TTY detection for the renderer
  probes stderr, color detection for logs probes stdout.
- **Environment resolution has two idioms**: `os.Getenv` sees only the
  process environment; `project.Environment[...]` also sees the project's
  `.env`. `cmd/compose/compose.go:setEnvWithDotEnv` additionally copies
  `COMPOSE_*` keys from the project's env-files into the process environment
  during `PersistentPreRunE` — variables read before that point (e.g. as
  cobra flag defaults) do not see the `.env`.
- **Container/project identity is label-based**: listings filter on the
  presence of `com.docker.compose.config-hash` (see
  `pkg/compose/containers.go:getDefaultFilters`), not just the project label.
  One-off (`compose run`) containers carry `oneoff=True` and no
  container-number; lifecycle-hook helper containers carry neither and are
  therefore invisible to `ps`/`down`.
- **Docker Desktop integrations** (`internal/desktop`, used from `up`,
  `publish`, the keyboard menu) talk to Desktop over a private socket and are
  expected to fail silently on non-Desktop engines.

## Lint

- Linter: golangci-lint v2 (config in `.golangci.yml`)
- Run: `golangci-lint run --build-tags "e2e" ./...`
- **After modifying any Go code, ALWAYS run the linter and fix all reported issues before considering the task complete.**
- Lint is also run via Docker: `docker buildx bake lint` (uses version pinned in `Dockerfile`)

## Code Style

- Formatting is enforced by golangci-lint (gofumpt + gci)
- Import order: stdlib, third-party, local module (enforced by gci)
- Max line length: 200 chars
- Max cyclomatic complexity: 16
- No `io/ioutil`, `github.com/pkg/errors`, `gopkg.in/yaml.v2`, `golang.org/x/exp/maps`, `golang.org/x/exp/slices`
- Use `github.com/containerd/errdefs` instead of `github.com/docker/docker/errdefs`
- In tests: use `t.Context()` instead of `context.Background()` or `context.TODO()`
- Prefer `fmt.Fprintf` over `WriteString(fmt.Sprintf(...))`

## Git

- **All commits MUST be signed off (DCO)**. Always pass `--signoff` (`-s`) to `git commit` and `git commit --amend`.

## Issue and PR Guidelines

- Never create an issue or pull request without the user's explicit instruction.
- If a user asks you to create a pull request on their behalf, add a file named
  `AI_AGENT_DISCLOSURE.md` to the change containing the text below followed by
  the current date (`YYYY-MM-DD`). If the file is already present in the
  repository, update its date to the current date so the disclosure is always
  part of the change:

  > *"This contribution was prepared by an AI agent acting on a human's behalf.
  > The human submitter may not have independently reviewed or tested the change."*

- If a user asks you to create an issue on their behalf, prepend the following
  line to the issue body:

  > *"This issue was filed by an AI agent on a human's behalf. The human
  > submitter may not have independently verified the report."*
