# Development workflow

1. Read the root entry point, this index and guides relevant to the task.
2. Inspect existing implementation, callers, tests, fixtures and docs. Distinguish
   confirmed defects from missing verification; do not rebuild working features.
3. Define expected behavior and a concise execution plan. Identify CLI, JSON,
   configuration, storage compatibility and security implications.
4. Establish the baseline. Follow [TDD](testing.md) for functionality and fixes.
5. Implement the smallest useful increment; run targeted tests frequently.
6. Assess and update affected documentation immediately after each meaningful
   change. Commit coherent units using [commit guidance](commits.md).
7. Run final validation, inspect the complete diff and remove accidental artifacts.
8. Report changes, tests actually run, failures and limitations. Never hide a
   failed check or describe an unrun check as successful.

Prefer Go's standard library and minimal dependencies. Preserve existing CLI
interfaces and output types. Keep domain logic separate from rendering, errors
explicit and behavior deterministic. Avoid speculative abstractions and unrelated
refactors. Treat recordings, output and synthetic test fixtures as security
boundaries: never use real credentials, weaken defaults, or log runtime values.

Work autonomously within the authorized task. Escalate genuinely destructive or
security-sensitive decisions when needed; ordinary reversible implementation and
verification do not require repeated confirmation. Publishing, merging and
release creation require task authorization. Do not bypass hooks or permissions.
