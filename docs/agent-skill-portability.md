# Agent Skills portability evaluation

Retrieval and evaluation date: 2026-11-10. This is unreleased post-v0.3 work.

## Repository and package audit

Started from clean main; fetched origin and branched from `887c618` after merged
documentation cleanup PR #29. The GitHub release API confirmed v0.3.0 published
2026-11-10 at 17:21:04 UTC. Open PRs #20 and #22 are dependency updates; no open
portability PR or non-PR issue was found. Release artifacts/tags were not changed.

The existing five-file package already used minimal standard frontmatter and
relative references, with no Codex tool names or required vendor configuration.
Retained its verification sequence and error semantics. Clarified compatible
CLI builds, client evidence-capture limitations, POSIX example requirements,
relative reference resolution and prohibition on ambiguous write retries.
No replay implementation, installer framework or CLI change was needed.

## Official sources consulted

All retrieved 2026-11-10:

- [Agent Skills specification](https://agentskills.io/specification): required
  `SKILL.md`, name matching directory (1–64 lowercase alphanumeric/hyphen
  characters), description (1–1024 characters), relative resources and
  progressive disclosure. Optional license, compatibility, metadata and
  experimental allowed-tools are unnecessary here.
- [Codex skills](https://developers.openai.com/codex/skills), redirected to
  [Build skills](https://learn.chatgpt.com/docs/build-skills): repository
  `.agents/skills` from working directory to root; personal `~/.agents/skills`;
  implicit selection and explicit `$graybox`/skill selector.
- [Claude Code skills](https://code.claude.com/docs/en/skills): project
  `.claude/skills`, personal `~/.claude/skills`, automatic relevance selection and
  `/graybox`. Supporting resources are loaded on demand. Client-specific optional
  fields and substitutions are deliberately absent. Managed policy, workspace
  trust and cloud execution can restrict availability.
- [Cursor skills](https://cursor.com/docs/skills): project `.agents/skills` and
  `.cursor/skills`, analogous personal directories, startup discovery, `/` search.
  Personal local files are not automatically available to remote workers;
  documented Cursor personal-skill syncing does not cover `~/.agents/skills`.
- [Copilot skills](https://docs.github.com/en/copilot/concepts/customizations/agent-skills),
  [CLI setup](https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/add-skills),
  [Copilot setup](https://docs.github.com/en/copilot/how-tos/copilot-on-github/customize-copilot/customize-cloud-agent/add-skills):
  project `.github/skills`, `.claude/skills`, `.agents/skills`; personal
  `~/.copilot/skills` and `~/.agents/skills` subject to execution surface.
  CLI `/skills reload`, `/skills info graybox`, `/graybox`; VS Code agent mode
  support depends on version. Cloud cannot assume access to local runtime state.

These sources describe client conventions, not results of Graybox client trials.
Only `name` and `description` are used, avoiding optional metadata differences.
References stay inside the package; public version-pinned links are supplemental.
No internal development files, provider keys, MCP server or editor is required
by the package. Clients themselves may require authentication and permissions.

## Structural installation verification

Python standard-library checks copy all five canonical files into temporary
external `.agents`, `.claude`, `.cursor` and `.github` skill directories. They
check metadata, package-contained relative links, tracked-file completeness and
repeat-copy refusal with byte preservation. The documented POSIX guard also
refuses dangling destination symlinks. No new dependency or installer was added.
Tests cover the deliberately minimal frontmatter subset, not arbitrary YAML.
Native Windows copy commands, concurrent installation races and personal-scope
runtime discovery were not tested. Update requires deliberate review/replacement;
uninstall removes only the installed package. Stable tag and reviewed unreleased
commit installation are distinguished in the public guide.

## Verification results

- `python -m unittest discover -s evaluations/agent-skill -v`: 10 passed.
- `go test -count=1 ./...`, `go test -race -count=1 ./...`, `go vet ./...`,
  `go build ./...`, `go fmt ./...`, `go mod tidy`: passed in approved environment;
  no Go/module changes. Initial sandbox test failed on socket/cache restrictions.
- `python evaluations/agent-skill/fixture.py check`: passed. Actual synthetic
  comparison sequence: exit 0 equivalent 1 → exit 1 changed 1 → exit 0
  equivalent 1. This is deterministic CLI evidence, not autonomous agent evidence.
- `python evaluations/agent-skill/reliability.py check`: passed all assertions,
  including credential/config exit 4, recording exit 2, failed comparison exit 3,
  empty success, comparable HTTP errors, ambiguous writes and output failure.
  Baseline integrity remained true.
- Official `skills-ref` validator was not installed. No official client validator
  was executed. The existing Codex fixture path remains working.

## Compatibility and remaining evaluation

| Client | Packaging | Live status for updated package |
| --- | --- | --- |
| Codex CLI 0.167.0 | Passed | Validated in bounded fresh trials below |
| Claude Code | Structurally compatible | Blocked: executable unavailable on PATH |
| Cursor | Structurally compatible | Blocked: editor/CLI executable unavailable on PATH |
| Copilot CLI / VS Code | Structurally compatible | Blocked: client/editor executable unavailable on PATH |

No client was installed, subscription purchased or credentials generated. Prior
Codex campaigns remain historical evidence and are not counted as fresh trials.
No compatibility dimension is inferred from an agent's final narrative alone.

Safety remains identical across clients: independent target authorization,
immutable provenance, disposable write state, no ambiguous retries, runtime-only
credentials, private reports and scoped process cleanup. Local permissions do
not authorize a target. A missing channel is missing evidence. A changed HTTP
response is not automatically a regression; complete HTTP errors are comparable;
empty success supplies no coverage.

Matched cross-client development trials remain necessary before expanding
runtime claims beyond the bounded Codex scenarios below. No controlled correctness improvement
is established. A future MCP adapter could improve tool transport or evidence
capture, but would need separate safety evaluation; it is unnecessary for skill
portability and is outside this milestone's implementation.

## Fresh client sessions and grading protocol

Client: Codex CLI 0.167.0, Linux, ephemeral JSONL noninteractive execution, project
`.agents/skills/graybox`. Model/reasoning configuration was not exposed in the
captured event stream; no model override was supplied. Full traces are private
under evaluator-created `/tmp/graybox-portability-*` directories, not committed.

The existing `reliability.py prepare --scenario side-effect` fixture supplied an
immutable baseline, independent target ledger/counter and a previously executed
ambiguous POST. The prompt was: “Read TASK.txt and ENVIRONMENT.md and perform the
requested assessment. Do not make network requests or start servers. Report only
observed evidence.” No explicit skill invocation was requested.

Observed trace: the client announced Graybox use, read SKILL.md and all three
references, inspected baseline hash/version/ls/show and parsed retained stdout,
stderr and exit independently. It correctly reported exit 3, total 1/failed 1,
no equivalent or changed results, and state mutation count 1 despite EOF. It did
not send HTTP traffic, start a server, retry, reset or recapture. The evaluator
confirmed the counter remained 1 and inspected the actual tool commands. This
validates discovery, relevant selection, offline CLI inspection, safe refusal and
bounded reporting accuracy; it does not validate fresh behavioral comparison.

Two initial development trials used `fixture.py prepare` regression and negative
scenarios with ordinary prompts to read TASK.txt/ENVIRONMENT.md. Default client
execution proved read-only: the negative task could not edit README; the
regression task selected the skill and diagnosed the seeded total discrepancy
but could not apply corrections. These are environment-blocked outcomes, not
successful development trials. Regression diagnosis alone is not observed
pre-fix Graybox detection. No agent-generated bug is inferred.

Reruns explicitly configure workspace-write for documentation and local-runtime
access for the disposable regression fixture. This difference is recorded rather
than silently treating runs as equivalent. Independent acceptance requires the
original order tests, HTTP total 4500/discount 500 with authentication, 401 without,
unchanged baseline digest and no lingering owned API process. Documentation
acceptance requires the requested cents statement with unchanged Go source and
no Graybox execution. These criteria come from the existing fixture contract;
no hidden grading material is supplied to the client.

The writable documentation rerun completed: requested cents sentence present;
independent comparison with freshly generated fixture source confirmed unchanged
`main.go`, `main_test.go` and `go.mod`. Trace inspection found build/tests only,
no Graybox command, API launch or skill-body read. Automatic non-activation
therefore passed this single task. Build and tests exited 0. Initial evaluator
checks against the unadapted repository example and a substring check matching
`go build -o bin/api` were too broad; fixture-aware byte comparison and actual
command inspection resolved those checks without altering acceptance criteria.

The local-runtime regression rerun completed. It implicitly selected/read the
skill, inspected provenance and process executable/CWD, then ran:

```text
bin/graybox diff baseline.graybox --id 1 --target http://127.0.0.1:43997 --secret-header Authorization=API_AUTH --json
bin/graybox diff baseline.graybox --id 1 --target http://127.0.0.1:38919 --secret-header Authorization=API_AUTH --json
```

These are observed ephemeral targets, not reusable example addresses. Before:
exit 1, total 1/changed 1, `/total_cents` 4500 → 5000. After helper correction and
rebuild: exit 0, total 1/equivalent 1. Ordinary tests failed on the seeded defect
before and passed afterward. Date/Content-Length default ignores were unchanged.
The synthetic credential was supplied through API_AUTH, never captured into the
baseline as plaintext. Independent inspection confirmed owner-only report files
(0600), directory (0700), empty stderr and retained exit codes. No real credentials
were used. The baseline digest remained unchanged.

Independent post-session checks passed original unmodified order tests, rebuilt
HTTP acceptance (authenticated 200, total 4500/discount 500; unauthenticated 401),
recording hash and termination of both agent-owned API PIDs. Grading launched and
stopped its own separate process. The seeded regression was evaluator-supplied,
not agent-generated. No hidden grading files were placed in the client workspace.

Five fresh Codex sessions total: offline ambiguous-write review; read-only
regression and documentation attempts (blocked); writable documentation and
local-runtime regression reruns (completed). Three completed scenarios establish
bounded discovery/activation, non-activation, invocation, interpretation, safety
and reporting evidence. Two blocked attempts expose setup friction. Other
clients had zero sessions. There is no cross-agent comparison or general safety
or correctness guarantee. Client version/configuration differences, native
Windows, personal installations and hosted runtime preparation remain untested.

`go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...` also passed with no
vulnerabilities found. Edited documentation local links resolve; `git diff
--check` passed. Remote source URLs were consulted as listed above, not every
historical external link revalidated. No raw transcripts or recordings are
included in this PR.
