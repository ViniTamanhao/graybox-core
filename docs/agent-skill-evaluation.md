# Agent skill evaluation

## Scope and acceptance criteria

Milestone 2 evaluates whether a coding agent selects Graybox for relevant HTTP
changes, preserves trustworthy baselines, checks target safety, interprets the
existing JSON contract, and reports only observed evidence. It does not change
the CLI or implement an agent framework.

The base is main `c9b5ddd4510d2deea8168769ddce6524a570f33d`: PR #24 was
confirmed merged on 2026-11-08. The latest published release checked was v0.2.3;
this work uses a source build containing the Milestone 1 contract.

## Evaluation layers

1. Structural: frontmatter, portable package links and isolated installation.
2. Deterministic: disposable application, actual CLI capture/inspection/diff,
   exit codes, JSON locations, immutable baseline and safe refusal cases.
3. Autonomous: fresh Codex sessions, installed skill, ordinary task prompts,
   inspected tool traces, code changes and final responses. Scripted commands
   are never counted as autonomous experiments.

Seven scenarios: preserving refactor, accidental regression, intentional change,
missing baseline, unsafe replay, missing credentials/failed comparison, and a
documentation-only negative control. Each trial is scored on discovery,
selection, safety, execution, interpretation, correction, reporting and restraint;
not applicable dimensions are marked N/A. A missing trace is unverified, not pass.

## Packaging research

The installed reference CLI is `codex-cli 0.167.0`. Its `exec --help` provides
`--cd`, `--json`, `--ephemeral`, `--sandbox` and `--output-last-message` for
separate sessions. Official [skill documentation](https://learn.chatgpt.com/docs/build-skills)
was checked on 2026-11-08: `SKILL.md` with YAML `name` and `description` is
required; project discovery uses `.agents/skills` from CWD to repository root,
user discovery uses `~/.agents/skills`. Description matching allows implicit
selection; `$graybox` permits explicit invocation. Optional `agents/openai.yaml`
is unnecessary for this instruction-only package. No extra configuration is
required. Restart if discovery has not refreshed.

Implementation and experimental results are recorded below as they are verified.
