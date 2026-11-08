# Verification workflow

Inspect → verify baseline → check safety → change code → run tests → diff →
interpret → report. Use the installed binary, not `go run`, for reliable process
exit codes. Full maintained [contract](https://github.com/ViniTamanhao/graybox-core/blob/main/docs/agent-verification-contract.md).

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

## Execute safely and retain channels

After safety review, supply the authorized full header through `API_AUTH` without
printing it. `--secret-header Authorization=API_AUTH` reads that variable.
For body/form/query mappings use supported `${ENV_VAR}` configuration references;
see [configuration](https://github.com/ViniTamanhao/graybox-core/blob/main/docs/configuration.md).
Configuration loads from CWD only or explicit `--config FILE`; inspect the policy.
Missing credentials are a blocker, not permission to guess.

Bash example (set `baseline`, `target`, and `exchange_id` to inspected values):

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
