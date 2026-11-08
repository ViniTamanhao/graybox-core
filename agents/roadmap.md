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
deterministic checks are separate from live agent evidence; empirical acceptance
remains subject to the recorded trial results and limitations. Subsequent milestones cover CLI
improvements driven by integration findings, end-to-end agent experiments, and
external validation/release.

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
