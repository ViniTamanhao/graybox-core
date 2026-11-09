# Milestone 4 protocol (fixed before trials)

Three tasks, conditions A (ordinary tools), B (CLI plus evidence), C (B plus
project-local skill). One run per cell, randomized with seed 403. Same source,
public tests, task text, configuration and 900-second limit per task. No inference
from exit zero alone. No rubric changes after trials. Linux-only launcher.

Tasks: extract pricing without contract changes; introduce LOYAL15 for subtotals
at least 10000 cents (15%, rounded half up), preserving existing rules; repair
cancellation of shipped orders and recover the stopped disposable service.
The last task starts with a deliberately seeded state-guard defect after capture;
never label it agent-generated. All tasks permit rebuilding/restarting only
owned fixture processes and resetting only their disposable database while stopped.

Independent black-box grading checks pricing boundaries, request validation,
authentication, cross-owner access, persisted creation/cancellation, shipped-state
rejection and required new behavior. Tests are not copied into agent workspaces.
Grade correctness as counts; critical auth/state violations separately. Count
unexpected failed contracts, not every JSON difference, as remaining regressions.
Detection/correction require actual trace evidence before completion and independent
final confirmation. Zero observed regressions gives no detection-rate denominator.

Score evidence, safety, reporting, recovery and discovery separately as Pass,
Fail, Blocked, Not observable or Not applicable. Normal tests alone may suffice;
never penalize non-adoption. Human CLI output is valid evidence; JSON/private
channels improve auditability but are not prerequisites for functional success.
Record relevant selection, immutable digest, state/reset/target checks, outcomes,
unsupported claims, elapsed time, tool counts and exposed token usage; cost is
unknown unless returned. No single composite score or statistical significance.

A/B measures CLI plus historical evidence, B/C incremental skill, A/C full workflow.
One implementer grades; no blinded human assessment or causal attribution. Preserve
neutral/failing results. Do not count setup probes as coding trials. All recordings,
transcripts and databases stay private outside Git. No production change without
independent defect reproduction. Repeated writes require stopped-service reset;
no automatic HTTP retries. Read-only comparisons do not establish transactional
rollback, dynamic ID correlation, concurrency safety or unrecorded coverage.
