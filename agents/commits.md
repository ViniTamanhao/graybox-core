# Commit conventions

Use imperative, descriptive Conventional Commit subjects:

```text
feat(scope): description
fix(scope): description
test(scope): description
refactor(scope): description
docs(scope): description
build(scope): description
ci(scope): description
chore(scope): description
```

Examples: `test(replay): reproduce missing credential failure`,
`fix(diff): preserve comparison failure semantics`,
`docs(agents): establish TDD development workflow`,
`ci(release): verify packaged binary versions`.

Commit progressively throughout the task. Prefer a regression test, matching fix,
optional refactor and related documentation as focused reviewable units. This is
an illustrative sequence, not a requirement to split every tiny edit. Do not save
a large implementation for one giant commit.

Keep commits independently buildable when practical. A committed RED test may
intentionally fail if clearly labeled and followed by its fix in the same PR;
the final PR must pass. Documentation should accompany or closely follow its
behavior change. Review staged diffs before committing: no secrets, recordings,
accidental binaries, generated noise, unrelated files or unnecessary formatting.
Use messages that describe what the commit actually does.

Never bypass Git hooks to obtain a commit. Do not rewrite published history or
force-push without explicit authorization. Respect user changes and branch scope.
