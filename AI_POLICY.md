# AI Usage Policy

Docker Compose has clear rules for AI-assisted contributions:

- **All AI usage in any form must be disclosed.** You must state
  the tool you used (e.g. GitHub Copilot, Claude Code, Cursor, ChatGPT)
  along with the extent that the work was AI-assisted.

- **Pull requests created in any way by AI can only be for approved issues.**
  Only submit PRs for issues labeled `status/approved` by a maintainer.
  Drive-by pull requests that do not reference an existing issue, or reference
  issues not yet approved (e.g., `status/0-triage`), will be closed. If AI
  isn't disclosed but a maintainer suspects its use, the PR will be closed.
  If you want to work on an issue, wait for maintainer approval first.

- **Pull requests created by AI must have been fully verified with
  human testing.** AI must not create hypothetically correct code that
  hasn't been tested. You must run `make test`, `make lint`, and relevant
  E2E tests locally. Importantly, you must not allow AI to write
  code for platforms or environments you don't have access to manually
  test on.

- **Code must follow Docker Compose's existing patterns.** Before writing
  code, read [AGENTS.md](AGENTS.md) and search for similar functionality
  in the codebase. AI-generated code that ignores project conventions
  (error handling, logging, testing patterns) will be rejected. Run
  `gofmt -s` before submitting.

- **Issues and discussions can use AI assistance but must have a full
  human-in-the-loop.** This means that any content generated with AI
  must have been reviewed _and edited_ by a human before submission.
  AI is very good at being overly verbose and including noise that
  distracts from the main point. Humans must do their research and
  trim this down.

- **Contributors who repeatedly ignore this policy will have PRs closed
  and may be banned from the repository.** We welcome developers at all
  skill levels and are happy to help you learn. But if you're learning,
  we encourage you to write code yourself rather than relying on AI—we'll
  provide better feedback that way.

These rules apply to all outside contributions. Maintainers may use AI
tools at their discretion, applying the judgment earned through their
contributions to the project.

## There are Humans Here

Please remember that Docker Compose is maintained by humans.

Every discussion, issue, and pull request is read and reviewed by
humans. It is a point of interaction between people and their work.
Approaching this with low-effort, unverified submissions is disrespectful
and puts the burden of validation on maintainers who volunteer their time.

In a perfect world, AI would produce high-quality, correct code every time.
But that reality depends on the person using the AI. Today, we see too many
contributions where AI-generated code hasn't been tested, doesn't follow
project patterns, or solves problems that don't exist. Until this improves,
we need clear rules to protect maintainer time.

## AI is Welcome Here

Docker Compose is developed with AI assistance, and many maintainers use
AI tools productively in their workflow. As a project, we welcome AI as
a tool for those who use it responsibly!

**Our reason for this policy is not an anti-AI stance**, but rather a
response to the increase in low-quality AI-generated pull requests that
don't address real user needs or follow project standards. It's about
the quality of contributions, not the tools used to create them.

This section exists to be transparent about the project's use of AI and
to clarify that this policy targets contribution quality, not the use
of AI tools themselves.