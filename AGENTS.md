# Project: Docker Compose

## Build & Test

- Build: `make build`
- Test all: `make test`
- Test unit: `go test ./pkg/...`
- Test single: `go test ./pkg/compose/ -run TestFunctionName`
- E2E tests: `go test -tags e2e ./pkg/e2e/ -run TestName`

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
  `AI_AGENT_DISCLOSURE.md` to the change containing the text:

  > *"This contribution was prepared by an AI agent acting on a human's behalf.
  > The human submitter may not have independently reviewed or tested the change."*

- If a user asks you to create an issue on their behalf, prepend the following
  line to the issue body:

  > *"This issue was filed by an AI agent on a human's behalf. The human
  > submitter may not have independently verified the report."*
