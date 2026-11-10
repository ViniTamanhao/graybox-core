# Verification workflow

Inspect → verify baseline → check safety → change code → run tests → diff →
interpret → report. Use the installed binary, not `go run`, for reliable process
exit codes. Full maintained [contract](https://github.com/ViniTamanhao/graybox-core/blob/v0.3.0/docs/agent-verification-contract.md).

## Establish evidence before editing

Find the project's recording and provenance: reference revision/build, capture
owner/time, environment/state, relevant requests and protection policy. Preserve
a digest and keep the artifact immutable. A filename or successful status alone
is not provenance. Inspect privately:

```bash
graybox version --json
graybox ls "$baseline" --json
graybox show "$baseline" 1 --json
```

Choose actual IDs from `ls.exchanges[].id`; inspect `method`, `path` and `status`.
`show` has `request` and `response`, each with headers and body. Check
`proxy_error`, body `complete` and `truncated`; decode `data` using `encoding`
(`utf8` or `base64`). Empty/unrelated selections establish no coverage.
`ls --path` only filters inspection; it does not select a later diff.
`--id` selects one positive ID; absent `--id`, all exchanges execute.

If no baseline exists, capture before modification only when the current state
is accepted and safe. Use a fresh private output file:

```bash
graybox record --target "$target" --output "$baseline" --json
```

Send authorized representative requests through the recorder, stop gracefully
(SIGINT/SIGTERM), check final exit/stderr for persistence loss, then inspect.
Startup JSON is not completion. A post-change recording cannot establish
historical equivalence. If reference state is unavailable, report unexecuted
comparison and proceed with ordinary tests.

When investigating a suspected regression or resuming a partially completed
refactor, compare the current build against the trusted baseline **before fixing**
the suspected difference when safe. Preserve the observed delta, then correct and
rerun. A final equivalent result alone does not show which defect was detected;
if an earlier comparison was blocked or omitted, state that limitation.

## Execute safely and retain channels

After safety review, supply the authorized full header through `API_AUTH` without
printing it. `--secret-header Authorization=API_AUTH` reads that variable.
For body/form/query mappings use supported `${ENV_VAR}` configuration references;
see [configuration](https://github.com/ViniTamanhao/graybox-core/blob/v0.3.0/docs/configuration.md).
Configuration loads from CWD only or explicit `--config FILE`; inspect the policy.
Missing credentials are a blocker, not permission to guess. If this already
prevents authorized comparison, record it as not executed; do not start a server
solely to manufacture an additional failure reason.

The following examples require a POSIX shell, `umask`, `mktemp` and a writable
private temporary directory. On other platforms use equivalent native tooling
with owner-only access, separate stdout/stderr and the actual process exit code.
If the client cannot provide these, report that limitation. Resolve package
references relative to this installed skill, not the application working directory.

Use this capture pattern for each comparison, with `baseline`, `target` and
`exchange_id` set to inspected values. Create the directory before redirection;
`mkdir -p /tmp/fixed-name` is not a substitute for fresh private storage:

```bash
umask 077
report_dir="$(mktemp -d /tmp/graybox-reports.XXXXXXXXXX)" || exit 1
if graybox diff "$baseline" --id "$exchange_id" --target "$target" \
  --secret-header Authorization=API_AUTH --json \
  > "$report_dir/diff.json" 2> "$report_dir/diff.stderr"; then
  result=0
else
  result=$?
fi
printf '%s\n' "$result" > "$report_dir/exit-code"
printf 'Graybox exit: %s\n' "$result"
```

Omit credential mapping for an endpoint that genuinely requires none. Keep files
private and outside version control; review before sharing, delete when no longer
needed. Diff sends requests; a missing report does not prove no effects occurred.

## Interpret both status and JSON

| Diff exit | Interpretation |
| --- | --- |
| 0 | Equivalent or empty; require relevant nonempty evidence |
| 1 | One or more changed, none failed; classify using task requirements |
| 2 | Invalid/unsupported recording or missing ID; unverified |
| 3 | At least one failed comparison; aggregate inconclusive |
| 4 | Arguments/config/credentials/target policy failure; unverified |
| 5 | Internal/filesystem/output failure; unverified |

On 0/1/3 require a complete JSON object with `summary` integer fields `total`,
`equivalent`, `changed`, `failed`, plus `results` and `ignored` arrays. Require
`total = equivalent + changed + failed = len(results)` and counts agreeing with
outcomes and exit. Unknown outcomes, inconsistent counts, malformed or absent
JSON are inconclusive. On 2/4/5 inspect stderr; there is no JSON error envelope.
Do not parse human diagnostic text as stable machine error categories.

Results identify `exchange_id`, `method`, `path`, `outcome`, `differences` and an
`error` for failure. Differences have `kind`, `location.component` and
`location.path`, and `before`/`after` objects with `present` and `value`.
A JSON field uses component `response.body` and RFC 6901 pointer such as
`/total_cents`; status uses `response.status` and an empty path. Preserve exact
JSON numbers and distinguish absent values from null. Failure dominates changes,
even when a failed result retains status/header differences.

A complete 401/403/500 is comparable HTTP behavior, not a transport failure.
Investigate authentication/state before blaming code. Equal HTTP errors may be
`equivalent` but do not verify a successful business path. Replay success alone
is never equivalence. Ignore Date/Content-Length defaults and redaction narrow
the claim; protected values and unrecorded paths are not verified.

For unexpected differences, inspect the affected handler and requirements; fix
and repeat with the original baseline and policy. For intended changes, report
what changed and why it is expected. Do not automatically replace baselines.
Rerun after relevant code/config/build changes, with side effects reassessed.
Report version/commit, baseline provenance/digest, independently verified target,
IDs/counts, exit, policy and locations, plus unverified behavior. Never claim an
empty success or an unexecuted command verifies preservation.


## Reconcile evidence before reporting

Before drafting the final answer, review this session's actual tool results and
make a short evidence ledger (in context, or inside the private report directory):

| Action | Execution evidence | Supported result |
| --- | --- | --- |
| Tests/build | Actual command, observed exit, returned output | Only the checks whose results were observed |
| Server startup/shutdown | Started PID/tool session and its output/status | Started, stopped, or an actual returned error; otherwise unknown |
| Graybox comparison | Actual command, exit, stdout/stderr paths, JSON parse/count check | Equivalent, changed or failed for identified exchanges; otherwise unverified |
| Skipped action | Not executed, with the authorization/prerequisite reason | No exit code, runtime error or comparison result exists |

Match **each factual clause** in the final response to a ledger entry. Remove
claims without supporting results. A planned command, expected sandbox restriction,
or another session's failure is not execution evidence. A tool denying permission
before launch means “not executed: tool permission denied,” not “the application
failed.” If a returned exit/channel is unavailable, mark it unknown; do not fill
it in from expectations. Ordinary test success never substitutes for a diff.

Use a compact final report: **Executed and observed**, **Not executed**,
**Observed differences** when present, and **Limitations/conclusion**. Combine
these into a few sentences when appropriate. Include baseline provenance/digest,
CLI version/source, target/build, selected IDs/counts, policy and exits for actual
comparisons. Quote only reviewed nonsensitive evidence. For each error claim,
check that its originating command and returned diagnostic are in this session.
Do not run an unauthorized or unnecessary command merely to support a report.

For example, if tests passed but credentials were unavailable: “Tests passed
(exit 0). Graybox comparison was not executed because no runtime credential was
authorized. API equivalence remains unverified.” Do **not** add “server startup
failed because sockets were denied” unless startup was actually attempted and
that diagnostic was returned. A predicted restriction, if relevant at all, must
be explicitly labeled as an untested expectation.
