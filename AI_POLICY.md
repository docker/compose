# AI Usage Policy

This is the shared AI usage policy for Moby and Docker open source projects.
It sets the baseline for AI-assisted issues, pull requests, and discussion
across repositories.

- **Disclose AI usage.** Say what you used: Copilot, Claude Code, Cursor,
  ChatGPT, whatever, and roughly how much of the work it did, in the issue
  or PR description. If it's not disclosed and a maintainer suspects
  otherwise, expect the issue or PR to get closed.

- **Skip the AI attribution trailer.** Disclosure belongs in the PR
  description, not the commit message. Add a `Co-Authored-By:` trailer for a
  human who actually worked on the change, not for the tool you used to
  write it. Whoever opens the PR signs off on it and owns it, no matter what
  wrote the first draft.

- **Review before you post.** Anything AI helped produce (an issue, a PR
  description, a review reply, a code change) gets read and edited by a
  human before it goes out. Using AI to draft a reply is fine, including to
  bridge a language gap. Posting raw, unreviewed output isn't. Understand
  it, check it's right, and own it before it goes out under your name.

- **Keep it short.** Lead with a plain-language summary of the problem or
  change. If there's real detail worth keeping (logs, reasoning,
  alternatives you tried), put it in a collapsible section instead of a
  wall of text:

  ```markdown
  <details>
  <summary>Details</summary>

  ...supporting detail here...

  </details>
  ```

  Some AI-written reports do contain useful detail; the point is to surface
  the actionable part, not to strip the detail out.

- **Fix the bug, not the file.** Don't fold in drive-by refactoring, typo
  fixes, or reformatting just because AI made them easy to generate. Spotted
  something else worth fixing? Open it separately. Smaller diffs review
  faster and carry less risk.

- **Landing the PR isn't the finish line.** If your change causes a
  regression, or draws a follow-up question, show up for it. A PR that
  exists to pad a contribution history, with no intent to follow through,
  isn't the kind of contribution we want.

- **Repeated violations have consequences.** Maintainers can close
  submissions that ignore this policy, and repeat offenders get restricted
  or banned.

### No contribution farming

Contribution farming is forbidden. Do not submit pull requests in an attempt
to inflate contribution counts, build a public portfolio or gain repository
activity rather than to improve the project.

Examples include:

- opening pull requests without verifying the problem or testing the
  proposed solution;
- submitting an AI-assisted pull request for a previously unreported problem
  without first opening an issue, allowing maintainers to verify the need, and
  receiving their approval to proceed;
- splitting one logical change into multiple trivial pull requests without a
  clear motivation;
- submitting mechanical, cosmetic, generated, or speculative changes without
  a concrete user or maintenance benefit.

Maintainers may choose to close such submissions without detailed review or
even ban contributors who repeatedly engage in this behavior.

This rule is based on submission quality and behavior, not contributor
experience or tool choice. First-time contributors and appropriately scoped
small fixes are welcome when they address a real, verified problem or need.

### Approved issues for AI-assisted PRs

Some repos only accept AI-assisted PRs against an issue a maintainer has
already triaged and approved: no speculative PRs built straight from an
AI-generated idea. This policy doesn't mandate a specific label or workflow;
repos are free to adopt it and tighten it, for example requiring a
`status/approved` label before AI-assisted work starts. Check the repo's own
`CONTRIBUTING.md` or `AGENTS.md` for what applies there.

These rules apply to outside contributions. Maintainers can use their own
judgment with AI tools, earned through their track record on the project.

## There are Humans Here

These projects are maintained by humans, on volunteer time.

Every issue and PR is read by a person, not a queue. A low-effort,
unverified submission dumps the cost of validation onto whoever picks it up.
AI doesn't change that: it just makes it easier to generate more of it,
faster.

We're writing these rules because we're seeing AI-generated content that
hasn't been checked, ignores project conventions, or solves a problem
nobody has. Until that changes, we need something explicit to protect
maintainer time.

## AI is Welcome Here

Plenty of maintainers on these projects use AI tools daily, and the
projects themselves are built with AI assistance. This isn't an anti-AI
policy.

It exists because of a rise in low-quality AI-generated contributions:
ones that don't address a real need or follow project conventions. The
target is contribution quality and accountability, not the tool that
produced it.
