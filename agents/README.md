# Agent development index

Graybox is a local-first API flight recorder and behavioral debugger:
record → inspect → change → diff → verify. Developers and agents use the same
CLI and engine; no account, AI service or cloud component is required.

Read this index before changes. Begin by checking the working tree, task scope,
relevant code, existing tests and documented contracts. Preserve user edits.
Write a short execution plan grounded in observed gaps, then iterate:

```text
Understand → Plan → Write failing test → Implement → Refactor
           → Verify → Update documentation → Commit → Repeat
```

## Guides

- [Development](development.md): task execution and completion requirements.
- [Testing](testing.md): RED → GREEN → REFACTOR and verification commands.
- [Commits](commits.md): focused history and Conventional Commits.
- [Documentation](documentation.md): update contracts incrementally.
- [Architecture](architecture.md): ownership and boundaries in the actual code.
- [Roadmap](roadmap.md): product scope, current capabilities and future proposals.

Explicit task instructions take precedence over these guides. Root AGENTS.md is
the repository entry point; these guides elaborate it. Existing public contracts
and security protections take precedence over speculative roadmap ideas. If two
documents conflict, inspect code/tests, identify the discrepancy and correct it;
do not silently weaken a published guarantee.

Before declaring completion, run targeted and practical broader checks, review
the complete diff for secrets/artifacts and compatibility, update documentation,
commit logical units, and report actual results and unverified areas. A passing
unit test alone does not prove a complete user workflow or release artifact works.
