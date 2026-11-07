# Incremental documentation

Every meaningful behavior change requires an assessment of affected docs.
Update them incrementally, not at the end of a large feature or at release time.

| Change | Update |
| --- | --- |
| CLI command/flag | Command help and relevant usage examples |
| Exit codes or JSON | docs/diffing.md and contract tests |
| Persisted representation | docs/recording-format.md and compatibility tests |
| Redaction or runtime credentials | docs/SECURITY.md and docs/configuration.md |
| Configuration | docs/configuration.md and examples |
| New user workflow | README or the relevant feature guide |
| Installation/build/release | docs/installation.md and contribution instructions |
| Development workflow | docs/CONTRIBUTING.md and agents/ guides |
| Product decision | agents/roadmap.md |

After each verified increment: identify affected documents, update existing
material rather than duplicating it, check examples against current command help,
and commit with the change or in a closely related commit.

Write accurate, concise, technically specific instructions organized around real
workflows. Show expected outcomes and exit codes where needed. Avoid marketing,
unsupported security claims and treating proposed roadmap features as shipped.
Report the platform/toolchain used for verification and what was not checked.
