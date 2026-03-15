# AI Usage Policy

This document defines the Docker Compose project’s policy regarding the use of AI-assisted tools (such as code generation, refactoring, documentation, or analysis tools) by contributors.

## Summary

- ✅ **AI tools are allowed** for contributing to Docker Compose.
- ⚠️ **AI-generated contributions are reviewed more strictly** than human-written ones.
- ❌ **Pull requests based on AI output will only be reviewed if they are explicitly approved in advance by a maintainer via a tracked issue and correctly tagged.**

The goal of this policy is to balance openness to modern tooling with the project’s standards for correctness, maintainability, and long-term stewardship.

---

## Allowed Use of AI

Contributors may use AI tools to assist with:

- Exploring ideas or approaches
- Writing or refactoring code
- Generating tests or documentation
- Explaining existing code or behavior

Using AI is **not considered misconduct**, and contributors are not required to avoid or hide AI usage.

---

## Requirements for AI-Assisted Pull Requests

Any pull request that includes **non-trivial AI-generated content** MUST meet **all** of the following requirements to be eligible for review:

### 1. Maintainer-Reviewed Issue

- The pull request **must be based on an issue that has been reviewed and approved by a Docker Compose maintainer**.
- The issue discussion must clearly establish:
    - The problem being solved
    - The expected behavior or outcome
    - The scope of the change

Pull requests created without this prior agreement **will not be reviewed**, regardless of quality.

### 2. Explicit Tagging

- The pull request **must be labeled with the tag**: