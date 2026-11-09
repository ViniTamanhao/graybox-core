# End-to-end agent development evaluation

Milestone 4 asks whether Graybox improves application correctness, regression
recognition or verification evidence over conventional development, and whether
its skill adds value beyond the CLI plus recordings. These are separate questions.
This is a small controlled campaign, not a statistically powered benchmark or a
release-readiness claim. No external developer participated.

## Baseline and predeclared design

Fetched `main` was `35ec2effbcefa6006f6f208dbc4bf50f77c22ee0`.
GitHub confirmed [PR #26](https://github.com/ViniTamanhao/graybox-core/pull/26)
merged on 2026-11-08. Its history includes Milestones 1–3. The starting tree was
clean. Existing CLI contracts, skill, reports and evaluation helpers were reviewed.
No production deficiency was assumed. The fixture/protocol were committed at
`7fa4810`; grading, capture and launcher at `870306f` before coding trials.

The [protocol](../evaluations/end-to-end/protocol.md),
[tasks](../evaluations/end-to-end/tasks.json) and
[reproduction guide](../evaluations/end-to-end/README.md) are the experiment inputs.
The application is an intentionally small Go commerce service using the existing
modernc SQLite dependency, integer cents, strict JSON input, runtime synthetic
credentials, owner authorization and persistent pending/shipped/cancelled orders.
Five endpoint types cover products, quotes, creation, reads and cancellation.
A stopped-server reset restores deterministic products/orders and next ID 43.
There are no payments, notifications, remote databases or external integrations.

| Task | Requested change | Starting state / grading distinction |
| --- | --- | --- |
| Refactor | Extract pricing into a helper/service and add tests | Accepted reference; preserve all public behavior |
| Feature | Add LOYAL15, 15% at subtotal ≥10000, half-up rounding, quote and creation | Accepted reference; intended historical rejection becomes success |
| Recovery | Reject shipped cancellation, preserve valid behavior, restore stopped local service | Deliberately seeded guard permits shipped cancellation after capture; never agent-generated |

A receives ordinary source/tests/tools. B additionally receives the reviewed CLI,
recordings, provenance and authorized state/credential context. C additionally
installs the unchanged reusable skill under `.agents/skills/graybox`. Task text
never mandates Graybox. B/C get extra historical evidence by design; A/B therefore
measures that bundle, not CLI software in isolation. Ordinary test success and
correct non-use of Graybox receive full functional credit.

Source, public tests, task text and Go module files were byte-matched across all
three conditions per task. Separate Git roots have no upstream history or remote.
A receives no Graybox binary, baseline or skill. Three tasks × three conditions;
randomized submission order with seed 403: feature B, recovery B, recovery A,
recovery C, refactor C, refactor B, refactor A, feature C, feature A. At most three
sessions run concurrently. CPU/network contention limits timing comparisons.

## Trustworthy recordings

Reference source passed black-box acceptance before capture. The actual Graybox
binary captured synthetic traffic through its reverse proxy; recorder shutdown
was observed with exit 0. Every exchange was inspected for complete/untruncated
bodies and absent proxy errors. Mandatory Authorization redaction was verified
by ensuring the generated credential bytes were absent. No custom sensitive
body fields were sent. Date/Content-Length defaults are the only ignored locations.
Recordings are private, uncommitted and never recaptured after agent changes.

| Set | Exchanges | State and authentication assumptions |
| --- | ---: | --- |
| reads | 3 | Product, own order, forbidden other-owner order; API_AUTH |
| quotes | 5 | SAVE10 normal/half-cent boundaries, unknown LOYAL15, invalid quantity; pure POSTs; API_AUTH |
| writes | 5 | Create/read deterministic ID 43, cancel pending, reject repeat, reject shipped; reset seed first; API_AUTH |
| unauth | 1 | Product rejection; **no** credential injection |

Each ordered write sequence is authorized only against its own resettable
fixture. No automatic retry or state correlation is implied. The independent
reference check reset before each set and observed four nonempty equivalent
reports. The seeded cancellation variant produced an actual changed comparison
on writes ID 5 and failed the independent state checks. A known feature variant
passed feature grading before trials. Provenance records source revision/hash,
binary hash, exchange identities, observed responses, policy and recording hashes.

## Independent acceptance and isolation

The evaluator's [grader](../evaluations/end-to-end/grade.py) uses real HTTP and
exact status/JSON contracts. It checks credentials, cross-owner reads/writes,
pricing and half-cent rounding, validation, old/required new discounts, creation,
persisted totals, cancellation, repeated/shipped rejection and state after process
restart. It adds feature-specific persisted creation checks. The public starting
test covers only four smoke paths; it is deliberately incomplete, not presented
as a comprehensive conventional suite. Agents may add any useful tests.

After each session, the evaluator builds its final source independently, resets
an owned database and grades without repairing source. Final narrative does not
override HTTP results. Refactor structure and actual test additions are reviewed
in source separately from HTTP preservation. Counts represent contract checks,
not independent statistical observations. Two failed checks for one state defect
are not counted as two distinct causal bugs.

Linux bubblewrap hides host `/home`, `/tmp` and `/run`, mounts only the trial's
source/private home and read-only toolchain/module cache, and uses a separate PID
namespace. Before every launch, a child verifies that the evaluator repository,
sibling campaign directory, grader and host Codex home are inaccessible. Thus
copying the grader into this repository does not expose it in trial workspaces.
No prior trial source or trace is mounted. Build caches are per-trial; the shared
module cache contains dependencies rather than task solutions. Networking is
permitted for inference and loopback use; it is not restricted to loopback by an
OS firewall. Task authorization remains fixture-only.

## Execution and setup observations

Linux x86_64, Go 1.27.3, Python 3.15.0, Codex CLI 0.167.0. Each real session used
`codex exec --ephemeral --sandbox workspace-write --json`, a 900-second limit,
network access enabled, and explicit `gpt-6-astra` / `medium` configuration matching
the host's selection. No backend identity or dollar cost is inferred from this
configuration. Authentication is provisioned privately, not in task text. C uses
real project skill discovery; reading the installed skill is checked in traces.
Official [noninteractive mode](https://learn.chatgpt.com/docs/non-interactive-mode)
and [skills](https://learn.chatgpt.com/docs/build-skills) guidance were consulted.

Setup probes are excluded from coding-trial counts. Restricted host sockets/cache
initially blocked preparation; authorized host execution resolved those failures.
One inference probe failed under restricted networking. Another timed out because
hiding `/run` hid the resolver file; a single-binary mount also omitted Codex's
code-mode helper. Mounting the resolved DNS file and companion binary directory
produced a successful real `pwd`/`go version` probe. A fresh campaign was then
prepared. These are evaluation-environment failures, not CLI defects or task failures.

The first campaign lacked `rg` in its isolated PATH; agents fell back to available
commands. They used writable `/tmp` Go caches because their tool sandbox did not
permit writes to the configured home cache. These common setup costs are included
in timing. No agent received a manual fix or solution during its task.

## Results

Nine real coding sessions completed on 2026-11-09, all within their deadline.
All nine passed independent acceptance: **267/267 checks, zero critical failures,
zero observed remaining unexpected regressions**. All three refactors extracted
pricing into a separate service file; all feature implementations made the same
small discount branch addition; all recovery implementations restored the accepted
guard. Agents added tests in every cell. No agent-generated application regression
was observed in source/trace review or grading; its detection rate is undefined.
This absence is not proof that untested paths have no bugs.

| Task | A: ordinary tools | B: CLI + evidence | C: CLI + skill + evidence |
| --- | --- | --- | --- |
| Refactor | 29/29; tests/build; no Graybox | 29/29; tests/build; chose no Graybox | 29/29; tests/build; 14 equivalent exchanges |
| Feature | 31/31; tests/build | 31/31; tests/build; chose no Graybox | 31/31; tests/build; 13 equivalent, 1 intended change |
| Recovery | 29/29; failing new test → fix → passing tests; live HTTP | 29/29; failing new test → fix → tests; 14 equivalent exchanges afterward | 29/29; actual pre-fix Graybox change plus failing new tests → fix → tests; 14 equivalent afterward |

All conditions total 89/89. A/B shows no correctness gain here. B/C shows increased
adoption (1/3 versus 3/3) and more structured historical verification, without a
correctness difference. A/C likewise shows additional historical evidence, not
better final application behavior. These observations cannot isolate a causal
skill effect from single-run model variation. Optional non-use was not penalized.

### Seeded, generated and intended differences

The seeded guard was detected and corrected in all three recovery cells. New
ordinary tests independently reproduced shipped/unknown-state cancellation in
all three. Only recovery C ran a **pre-fix** Graybox comparison: writes ID 5,
POST /orders/42/cancel, status 409→200 and rejection body→cancelled order. Its
report was exit 1, 4 equivalent / 1 changed / 0 failed. After the one-line fix,
all four sets returned exit 0 and 14 equivalent. The original smoke suite lacked
this case, so historical evidence added coverage beyond that suite; however,
agent-authored conventional regression tests also found it. This does **not**
establish unique detection beyond normal testing or discovery of an unknown bug.
Recovery B compared only after fixing; do not attribute its initial detection
to Graybox. A had no Graybox access and succeeded conventionally.

Feature C retained the intended change: quotes ID 4 changed 400→200, replacing
invalid_discount with subtotal 10000, discount 1500, total 8500. It observed exit 1
for quotes (4 equivalent / 1 changed), exit 0 for the other sets, and explicitly
classified the difference as requested. No baseline replacement or ignore
expansion occurred. The feature's new persisted-creation behavior was graded by
independent tests; its historical recording did not cover LOYAL15 creation.
There were no observed agent-generated regressions to detect or correct.

### Recovery and safety

All recovery agents rebuilt the application, reset only their own stopped
fixture database, started an owned server on a new OS-selected port and executed
verification. A used direct HTTP; B used four Graybox sets; C used owned before/
after processes with observed termination between reset and rerun. C then started
a fresh seeded development service, as did A/B for their final handoff. Their final
running-service statements matched startup traces at completion; the evaluator's
namespace teardown subsequently ended those services. They are not persistent
user-facing deployments. No stale-port probe, ambiguous-write retry, broad process
kill, authentication weakening or external application request was observed.

This is autonomous **restoration from a disclosed stopped service**, not recovery
from an unexpected mid-comparison outage. No trial demonstrated diagnosis of a
new transport failure, wrong running build or ambiguous partially committed write.
Those harder recovery/unsafe-replay cases remain unexecuted in this campaign.
Previous milestone deterministic tests are not substituted for live evidence.

| Dimension | A (three trials) | B (three trials) | C (three trials) |
| --- | --- | --- | --- |
| Required change / independent correctness | Pass, all | Pass, all | Pass, all |
| Relevant verification evidence | Pass: ordinary tests, recovery HTTP | Pass: ordinary tests; recovery historical comparison | Pass: ordinary tests plus historical comparisons |
| Skill discovery | Not applicable | Not applicable | Pass: all read installed SKILL.md and references |
| Comparison interpretation | Not applicable | Pass in recovery; otherwise Not applicable | Pass: equivalent, seeded difference and intended difference |
| Seeded correction | Pass in recovery | Pass in recovery | Pass in recovery |
| Credential / target / state safety in observed actions | Pass; replay Not applicable | Pass in executed recovery; other replay Not applicable | Pass in observed comparisons |
| Final factual claims | Pass in reviewed traces | Pass in reviewed traces | Pass in reviewed traces |
| Mid-operation failure recovery | Not observable | Not observable | Not observable |
| Raw report file modes after execution | Not applicable | Not observable: lost temporary files | Not observable: lost temporary files |
| Evaluator raw report retention | Not applicable | Fail for recovery | Fail for all three; corrected for future runs |

Safety scores apply only to observed authorized actions, not broad reliability.
B recovery used a fresh tempfile directory and saved stdout, but did not retain
separate stderr/status files. C used fresh private directories, separate channels/
status and JSON count checks, inspected requests, and verified baseline integrity.
Trace-visible creation under umask 077 supports intended protection; original
report modes cannot be independently rechecked after teardown. No guessed runtime
credentials were needed. All 24 treatment recording copies (four × six trials)
matched their original digests after execution. Generated runtime credential values
were absent from all nine JSONL traces, stderr and final responses. No agent
committed artifacts. A host /proc audit found no remaining campaign processes.

### Reporting accuracy and artifact loss

Final test/build claims matched returned command evidence. Graybox claims matched
actual tool output; no zero-exchange, false equivalence, invented failure or
unsupported regression claim was observed. Some shell chains returned only the
last command's status: test pass/fail text and subsequent build/use were inspected
rather than assigning invented individual numeric exits. Running-service claims
are valid only at completion before evaluator cleanup.

The first launcher used a temporary mount for /tmp. This erased agents' private
raw comparison files and made their report links unusable after namespace exit.
Private JSONL, final responses, source trees, grading, baseline bytes/hashes and
invocation metadata survived outside that mount. Traces contain actual comparison
counts and changed-result JSON; they are not replacements for every original raw
stdout/stderr file. **Full original report retention was not achieved.**

A deterministic regression test reproduced report disappearance (RED), then a
per-trial private scratch bind preserved the file across exit with 0700/0600 modes
(GREEN) while keeping evaluator/sibling files inaccessible. Commit `fe4d580`
fixes the evaluator only. It cannot recover lost original files and was not used
to retroactively upgrade retention scores. No CLI or skill change was warranted.

### Efficiency and evidence index

Command items count unique `command_execution` event IDs, including long-running
server starts; one item may contain several commands. They are not an exact count
of all internal tool calls. Token counts below are CLI-reported cumulative usage,
including cached input, not independently measured billing. Dollar cost is unknown.
Elapsed time covers launcher/session execution, excludes subsequent grading and
campaign preparation, and includes setup friction and shared-host contention.


| Trial | Seconds | Command items | Reported input / output tokens | Session | Trace SHA-256 prefix |
| --- | ---: | ---: | --- | --- | --- |
| refactor-A | 104.66 | 6 | 172654 / 2874 | `01a1c235-a48d-7aa0-9811-7367e2f451bf` | `9a4844a9209193a3` |
| refactor-B | 106.47 | 6 | 172408 / 3108 | `01a1c234-5503-7561-b36f-4a416323deb1` | `42e8e4120ce9c253` |
| refactor-C | 127.08 | 12 | 300336 / 3750 | `01a1c234-46a4-74e0-8320-1f027ceccf85` | `c0e209da3cc815a5` |
| feature-A | 85.33 | 4 | 147973 / 2251 | `01a1c236-392a-7e00-9c1d-907e2427d730` | `8412b7eea4091c08` |
| feature-B | 89.55 | 6 | 170190 / 2220 | `01a1c232-9937-7bf2-a32a-9cb72333efe4` | `54827fcc48b0bfb7` |
| feature-C | 98.22 | 11 | 335900 / 2627 | `01a1c235-f6fc-7db1-9523-66fa6bddbeef` | `7c9dde4108271a51` |
| recovery-A | 113.06 | 9 | 294670 / 2498 | `01a1c232-9939-7963-aeca-647c28381231` | `908ec8fa5f9401b0` |
| recovery-B | 109.40 | 11 | 379220 / 2535 | `01a1c232-9937-7832-9cfa-5f2430e920bf` | `a2f1d171265da22f` |
| recovery-C | 107.27 | 13 | 398710 / 3240 | `01a1c233-f99d-73d1-ab9d-f99f32d9bc41` | `d0b0d8b2867e2da7` |

Summed session time: A 303.05 s, B 305.42 s, C 332.57 s. Command items: A 19,
B 23, C 36. C used 29.52 more summed seconds than A in this sample; no causal or
statistical overhead estimate follows. Four trials made 17 actual diff invocations:
recovery B 4, recovery C 5 (including before fix), refactor C 4, feature C 4.
Preparation, isolation debugging, recording review and manual grading required
substantial evaluator effort not captured in session time; setup was not a free
input. A complete setup-cost benchmark was not instrumented.

Private evidence root for this host session: `/tmp/graybox-m4-p9a4rl05/trials`.
It is ephemeral, owner-only and uncommitted; hashes identify evidence, not permanent
availability. Baseline digest prefixes: reads `c0a536171cf98664`, quotes
`63ab2adc8c6e10c1`, writes `17ac81f065f67966`, unauth `fc514cbbe2ff1a2b`.
Reference main.go SHA-256 prefix `dc19e3eaef2ffa79`; Graybox binary
`77597155a6f00a14`; source revision `870306f0c46517c7f36ed9c9ecb3ee67acc5857c`.
Unchanged skill package SHA-256 `edc1a509a079fab49dc03bfc59b167fd480b42da861e423b53beee60e6f99619`
uses sorted relative paths, NUL and file bytes, as in prior milestones.

## Product implications and limits

**Tool value:** useful historical preservation evidence, but no improvement in
final correctness demonstrated. Controls solved all tasks. The initial smoke
suite was sparse, yet agents successfully extended it. Graybox detected the
seeded HTTP difference in C; conventional tests also detected that known bug.
No claim that Graybox uniquely caught a regression missed by adequate ordinary
testing is supported. No agent-generated mistake was available to evaluate.

**Skill value:** all C sessions discovered/read the installed skill and executed
structured comparisons; two B sessions completed without adopting the available
tool. C supplied request inspection, private separate channels and count validation.
These observed discipline differences are useful but do not establish a general
safety or correctness advantage. Historical comparison added overhead without
changing any final acceptance outcome. There is no reason to penalize the successful
B/A non-adopters or force replay for every task.

**Confirmed CLI defects:** none. **Skill defects:** none established requiring an
edit. **Evaluation defects:** DNS/helper mounts during setup, missing rg and home
cache friction, and lost raw temporary reports (fixed with a regression test).
**Capability boundaries:** explicit reset/order dependencies remain evaluator work;
no transactional rollback, dynamic IDs, stateful correlation, broad coverage proof
or safe automatic retry. These belong in later product investigations, not a new
Milestone 4 replay engine. Onboarding could better distinguish evidence setup cost
from running a comparison; v0.4 suite design may help only after concrete use cases.

Limits: one compact application, one model/configuration and host, one run per
cell, no blinded human grader, no external developer, no adversarial task, no
production traffic, no alternative platform, no unsafe replay refusal task, and
no unexpected mid-operation recovery. Grading was performed by the implementing
agent with an independently executed black-box suite, not an independent assessor.
The suite covers selected status/body/state contracts, not all headers, concurrency,
load, storage failures or arbitrary monetary products. Networks were not restricted
to fixture-only by a firewall. All nine final sources passed, creating a ceiling
effect that limits discrimination. Raw-report loss limits retrospective audit.
No paid task repeats were justified by failed or ambiguous correctness outcomes;
launcher probes and deterministic checks are separate evidence classes.

## Verification and next steps

Observed locally: fixture RED (501 handlers) → GREEN; independent reference,
seeded and known-feature grading; four equivalent baseline sets; seeded diff;
source/task matching; hidden-path isolation; integrity and credential scans;
report-retention RED → GREEN; process audit; and the following repository checks:

- `go fmt ./...` and `go mod tidy`: completed, no module or formatting changes.
- `go test -count=1 ./...` and `go test -race -count=1 ./...`: passed freshly.
- `go vet ./...`, `go build ./...`, pinned `govulncheck@v1.8.0`: completed; no vulnerabilities reported.
- Existing Python unittest suite: 8 passed; existing fixture and reliability checks passed.
- New runner deterministic `check` passed; new isolation/retention unittest passed.
- 30 local Markdown link targets and Bash example syntax checked; Python compilation
  and `git diff --check` passed. Skill-package checks passed unchanged.
- Vet and build were also rerun separately with observed exit 0 for each.

No production packages, dependencies, CLI/JSON interfaces, exit codes, mandatory
redaction or schema-1 recording format changed. Fixture code remains under
`evaluations/end-to-end`. Skill remains byte-identical. Full raw artifacts,
credentials and binaries are excluded from Git.

The bounded campaign and reproducible infrastructure are complete with the
explicit retention and coverage limitations above. The stronger hypothesis that
Graybox improves final correctness remains **unproven**, not a failed experiment
to hide. Human review should examine the protocol and ceiling effect before
accepting broader product claims. Milestone 5 should recruit authorized external
participants using the disposable guide, review setup costs, include harder
unknown-regression tasks and repeat balanced conditions. v0.3 release preparation
still needs its own compatibility/security/release checks and human review;
completion of this PR alone does not establish release readiness.
