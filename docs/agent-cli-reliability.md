# Agent-facing CLI reliability

Milestone 3 investigates failures without presuming a new CLI interface is needed.
Base: `2c39aa06a5e2e6c483f1ac3a31f4fb3a88d1e791` (fetched main);
PRs #24 and #25 were confirmed merged through GitHub on 2026-11-08.
This is source-build evidence, not a release announcement.

## Initial technical audit

| Surface | Existing guarantee and inspected coverage | Candidate gap / affected workflow |
| --- | --- | --- |
| Arguments/config/credentials | `root.go`, `config.go`, `secret_headers.go`; command channel, config applicability and secret tests: exit 4, stderr, preflight without traffic | Text diagnostics have no stable taxonomy; already documented; no demonstrated need for production changes |
| Recording/selection | Read-only storage schema/body checks; storage tests, `TestEmptyAndInvalidRecordingsCLI`, missing ID tests: exit 2; empty valid selection exits 0 | Empty success is explicitly no coverage; test agent interpretation |
| Target/reconstruction | Explicit target policy, no redirects, missing markers and incomplete requests refused; replay and hardening tests | Loopback does not establish isolation; retries require independent safety assessment |
| Execution | Sequential replay; cancellation/deadline/refused-connection/partial-stream tests | Transport failure cannot establish whether server state changed; add binary-level observable counter experiment |
| Comparison | Failed dominates changed; partial differences and later results preserved; `TestAgentDiffOutcomeContract` independently decodes JSON | Existing mixed test uses incomplete baseline; extend actual binary evidence to live stream failure |
| Output | Deliberate JSON DTOs, runtime scrubbing, output errors return 5; complete reports only after execution | Malformed/missing reports already documented as inconclusive; exercise consumer-side fault injection and real output failure |
| HTTP errors | Complete 401/403/500 are comparable; identical 401/500 can be equivalent | Business success differs from equivalence; already documented |
| Compact replay | `Execution.result()` omits status after read failure; explicit replay regression test | Contract incorrectly says status and error can coexist for body-read failure; documentation correction justified |

No production defect established by inspection. Current interfaces support safe
conservative classification, but cannot promise exact machine-readable causes,
server identity, rollback, coverage completeness or whether a failed request was
processed. Those limits are documented, not reasons to invent inferred facts.

## Investigation plan

Extend the existing `evaluations/agent-skill` helpers with nine controlled failure
scenarios, using built Graybox, private disposable files and synthetic local HTTP.
Capture status/stdout/stderr independently, decode with Python, check baseline
integrity and actual target observations. Keep deterministic assertions distinct
from independent Codex sessions. Prioritize mixed, incomplete, malformed report,
side-effect and unavailable-target trials; grade their traces using the same
criteria. Change production only after a failing behavioral reproduction.

Results, decisions and final verification will be appended after execution.
