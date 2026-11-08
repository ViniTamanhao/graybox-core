---
name: graybox
description: Verify observable HTTP API behavior with Graybox when refactoring handlers or middleware, changing serialization, authentication, pricing or REST endpoints, or fixing API regressions. Assess baseline provenance and replay safety before comparing. Skip documentation-only, styling and unrelated utility work.
---

# Graybox behavioral verification

Use Graybox when recorded HTTP evidence adds value to the requested coding task.
It supplements normal tests. A difference is not automatically a defect.

Prerequisites: a Graybox executable supporting `diff --json` and runtime secret
headers (Milestone 1 source contract), a trustworthy reference state or recording,
and an independently verified, authorized replay environment. If these are
missing, continue appropriate development/tests and report verification limits.
Do not install executables from untrusted sources.

1. Identify intended changes and behavior that must remain unchanged. Read
   [verification](references/verification.md) for commands and result semantics.
2. Before editing, locate and inspect relevant recordings and their provenance.
   If absent, capture only from an available accepted **pre-change** state with
   authorized requests. Otherwise report no trustworthy baseline. Never fabricate,
   overwrite or replace historical evidence to pass a comparison.
3. Read [safety](references/safety.md) before sending traffic. Inspect methods,
   payloads, completeness and authentication; establish target process/build and
   side effects independently. Loopback is not proof of safety.
4. Make the requested change, run normal tests, and start the intended build.
   Diff only relevant exchanges against the safe target. `diff` already replays;
   do not precede it with an unnecessary `replay`.
5. Capture exit status, stdout and stderr separately. Parse JSON and require
   nonempty relevant results with consistent counts. Distinguish equivalent,
   changed, failed, command failure and unexecuted verification.
6. Investigate unexpected differences by exchange and field. Fix actual
   regressions and rerun tests/diff when safe. Judge intended changes against
   requirements; never broaden ignores or weaken authentication to obtain a pass.
7. Report baseline provenance, CLI version/source commit, target/build, exchange
   IDs/counts, policy/ignores, exit code, reviewed changes and limitations. Confirm
   baseline integrity. Report only observed failures; distinguish predicted
   restrictions from errors returned by commands actually run. Review evidence
   for sensitive content before quoting it.

For blocked commands or ambiguous evidence, use
[troubleshooting](references/troubleshooting.md). For a synthetic authenticated
capture/change/restore exercise, use [the example](examples/authenticated-api.md).
The skill works in external projects without Graybox's development `agents/` files.
