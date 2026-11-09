# Disposable commerce development evaluation

Read [protocol.md](protocol.md) before running trials. This fixture is evaluation
code, not a production example or a new Graybox engine. Requires Go matching the
root go.mod, Python 3.9+, Git, and Linux bubblewrap for isolated live trials.
Live inference additionally requires an authenticated Codex CLI. No external
commerce services, notifications, payments or real customer data are used.

The Go fixture uses the repository's existing modernc SQLite driver. Products
1/2/3 cost 1999/5/10000 cents. Orders 40/41/42 belong to Alice/Bob/Alice and are
pending/pending/shipped. Creation starts at ID 43. SAVE10 rounds half up; quantities
are 1–100. JSON rejects unknown fields and trailing values. Runtime API_AUTH and
OTHER_AUTH identify the two synthetic owners; do not print their values.

GET /products/{id}, POST /discounts/apply, POST /orders, GET /orders/{id}, and
POST /orders/{id}/cancel use authenticated JSON. Quote POSTs are pure; creation
and cancellation mutate SQLite. No retry, rollback across HTTP calls, dynamic-ID
substitution, idempotency protocol or external integrations exist.

Build locally with `go build -o /tmp/commerce ./evaluations/end-to-end/app`.
With synthetic runtime headers provisioned in API_AUTH and OTHER_AUTH, run
`/tmp/commerce --db /private/path/commerce.sqlite --reset` while stopped, then
`/tmp/commerce --db /private/path/commerce.sqlite`. Startup prints its OS-selected
loopback URL. Stop only the process you launched and observe termination.
Reset drops this fixture's tables, so never point it at an unrelated database.

The runner prepares fresh private directories, captures reference traffic before
seeding defects, preserves provenance/digests and independently grades HTTP.
Never commit databases, recordings, binaries, raw reports or conversations.
Acceptance code remains evaluator-side, absent from trial mounts and Git history.
See the runner's help and evaluation report for executed commands and limitations.

## Deterministic verification and preparation

```bash
python evaluations/end-to-end/runner.py check
umask 077
campaign_parent="$(mktemp -d /tmp/commerce-campaign.XXXXXXXX)"
python evaluations/end-to-end/runner.py prepare --root "$campaign_parent/run"
```

`check` grades the reference over real HTTP including process restart persistence,
checks four captured sets against reset reference state, independently rejects the
seeded defect, observes it in a real diff, and grades a known feature implementation.
It removes its own temporary tree and stops its owned processes. No paid inference.
`prepare` refuses an existing output root. It creates nine pristine workspaces,
private runtime credentials, immutable baseline provenance and randomized order.
The reference traffic contains 14 exchanges: reads (3), pure quotes (5), ordered
writes (5), and unauthenticated rejection (1). Captures use mandatory header
redaction; only synthetic product/quantity/code fields are retained. Never inject
a successful credential into the unauthenticated set. Never run writes against
non-reset state. The recorder reserve/rebind port race fails explicitly.

## Actual agents

Use a separate preparation root for launcher probes; never count probes as trials.

```bash
python evaluations/end-to-end/live.py probe --root "$campaign_parent/run"
# Prepare a fresh campaign after the probe succeeds, then:
python evaluations/end-to-end/live.py run --root /path/to/fresh/campaign
# Or one predeclared cell:
python evaluations/end-to-end/live.py run --root /path/to/fresh/campaign --trial refactor-A
```

The launcher uses real `codex exec --ephemeral --sandbox workspace-write --json`
and `sandbox_workspace_write.network_access=true`, model gpt-6-astra/medium,
900-second per-trial deadline. Change model only before a new entire campaign.
It copies local Codex authentication privately; never publish that directory.
The launcher needs host network access to inference and local fixture sockets.
Networking is broader than loopback, but task authorization is fixture-only.
Official [noninteractive execution](https://learn.chatgpt.com/docs/non-interactive-mode)
and [skill discovery](https://learn.chatgpt.com/docs/build-skills) were consulted;
installed CLI help determines supported flags.

Bubblewrap mounts only that trial's source and private home over hidden host
`/home`, `/tmp`, `/run`, with a separate PID namespace. Go toolchain/module cache
are read-only; build cache is per-trial. Resolver file and Codex companion binaries
are mounted explicitly. The evaluator repo, sibling trials, hidden grader and
host Codex history are tested absent before invocation. C installs the unchanged
skill in `.agents/skills/graybox`; A/B have no copy. No baseline exists in A's
source, Git history or PATH. No GitHub remote is configured in a trial.

After namespace termination the evaluator builds and grades the final source
without altering it (only fresh binary/database outputs). JSONL, stderr, final
response, metadata, source diff, integrity and grading remain private. Review
actual command items and source changes manually; the launcher does not score
agent reasoning. Inspect any timeout/blocked cell before deciding on a fresh repeat.
Do not rerun a completed cell in place. Namespace teardown kills remaining owned
processes; agent cleanup itself must be scored separately from evaluator cleanup.
Reports written into the namespace's temporary `/tmp` disappear at teardown;
agent reports needed for later audit should live in its private home outside the
checkout. Tool output remains in JSONL. Retain evidence only as long as needed.

## External developer procedure

Build Graybox from the reviewed revision; prepare this disposable fixture and
choose C to install the reusable skill. Run an ordinary task, examine independent
grading and actual comparisons, then share only reviewed counts, setup friction,
and unexpected behavior. No production credentials or traffic are needed. No
external participant or independent human review is implied by this guide.
