# Product goals and roadmap

**Record what an API does. Change the code. Verify what changed.** Graybox provides
portable behavioral evidence for developers and AI coding agents. It remains
useful without AI: local-first, CLI-first, open-source core, single binary, no
mandatory account/cloud, minimal dependencies, composable machine-readable output
and deterministic behavior where possible.

## Current capabilities

HTTP reverse-proxy recording into SQLite schema-1 artifacts; inspect/list;
sequential replay; semantic JSON and exact-body diff; configurable ignores;
human/JSON output and exit codes; mandatory/custom redaction; runtime credential
injection and strict YAML configuration. See public docs for protection limits.

## Phase 1: stabilize the foundation

Reliability, release correctness, authenticated end-to-end tests, security
verification and onboarding. The [audit](../docs/stabilization-audit.md) records
evidence and limitations; passing tests do not establish universal safety.

## Phase 2: agent-native workflows (proposed v0.3)

Milestone 1 establishes the [verification contract](../docs/agent-verification-contract.md)
and [practical workflow](../docs/agent-verification-workflow.md), backed by CLI tests.
Milestone 2 adds a [reusable Codex skill](../docs/agent-skill.md) and
[reproducible evaluations](../docs/agent-skill-evaluation.md). Structural and
deterministic checks are separate from live agent evidence. Sixteen real Codex
sessions across two rounds include a matched no-skill control. The focused
reliability retest met its acceptance criteria: two factual credential-blocked
reports and a regression detected, corrected and verified with private reports
and scoped cleanup. Milestone 2 is ready for human review; earlier failures and
limits remain documented, without a general reliability or release-readiness
claim. Milestone 3 adds an [evidence-driven CLI reliability audit](../docs/agent-cli-reliability.md):
nine deterministic failure scenarios and five fresh Codex tasks support the
existing failure contract without a production change. Handoff review trials
do not establish autonomous recovery. Milestone 4 adds a [controlled end-to-end campaign](../docs/agent-end-to-end-evaluation.md)
with a disposable SQLite commerce API and isolated independent HTTP grading.
Nine real Codex sessions (three tasks × ordinary/CLI/CLI-plus-skill conditions)
passed all 267 acceptance checks. Skill-enabled sessions supplied more historical
verification, including a seeded pre-fix difference and an intentional change;
controls also succeeded, so improved final correctness is not established.
No agent-generated regression occurred in observed evidence. Raw temporary report
retention failed in the first campaign and the evaluator was corrected; traces
and final source remained private. External developer validation and harder
balanced tasks remain next steps, not completed release-readiness evidence.

Make it straightforward for agents to inspect recordings, replay requests, compare
responses, interpret structured outcomes and verify code changes. Favor existing
CLI commands and shared functionality for human and AI consumers. Agent skills
may guide workflows. An MCP server may be considered later, but is not required.

## Phase 3: suites and CI (proposed v0.4)

Reusable behavioral baselines and automated verification in CI. Define stable
suite contracts and useful failure evidence before adding infrastructure.

## Phase 4 and later (proposed v0.5)

Stateful replay, dynamic identifiers, environment-dependent values and safe
side-effect handling; richer protocols and advanced workflows only with clear
use cases. These are possibilities, not current guarantees or delivery dates.
All version labels above are provisional and are not published releases.

## Non-goals

Do not prioritize hosted observability, Kubernetes operators, AI chat interfaces,
mandatory cloud services, large dashboards, arbitrary network traffic capture or
unnecessary enterprise infrastructure. Possible commercial work (shared
baselines, CI history, organizational policy, team collaboration) must not
displace the free local CLI.

## Post-v0.3: Milestone 5.1 interoperability

Preserve one canonical Agent Skills package across Codex, Claude Code, Cursor
and Copilot. The [portability report](../docs/agent-skill-portability.md) separates
structural checks from live trials and unavailable clients. This is unreleased
work, not a change to v0.3.0 historical evidence. Complete matched live adoption
trials before broadening runtime claims. An MCP adapter could later expose CLI
operations, but needs separate authorization and credential design and is not
required for skills.
