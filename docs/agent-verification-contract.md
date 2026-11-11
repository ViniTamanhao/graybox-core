# Agent verification contract

This is the supported CLI contract shipped in v0.3.0 and retained in the published
v0.3.1 release. The separately installed skill guides use of this CLI. Use
`graybox version --json` and record the source commit for a `dev` build.
See the [workflow](agent-verification-workflow.md) for execution examples.

## Commands and channels

Flags belong to the command and may appear before or after its positional arguments.
Use a built executable to obtain Graybox's exit code directly (`go run` can wrap
nonzero process exits). Capture stdout, stderr and process status separately.

| Invocation | Purpose and JSON stdout |
| --- | --- |
| `graybox record --target URL --output FILE --json` | Long-running proxy; one startup object, **not** a completed recording report |
| `graybox ls FILE --json` | Inspect exchange identities; object with `recording` string and `exchanges` array |
| `graybox show FILE ID --json` | Inspect one stored exchange object, including body availability |
| `graybox replay FILE --json` | Execute requests; execution results, **not** an equivalence verdict |
| `graybox diff FILE --json` | Execute requests and compare recorded/live responses |
| `graybox version --json` | Object with `version` string |
| `graybox help COMMAND` | Human help; no machine schema |

Without `--json`, output is for humans; wording, spacing and abbreviations are
not parsing interfaces. `--help` also prints text even alongside `--json`.
Normal JSON reports are single objects, not JSON Lines or streamed results.
Command-level errors print text to stderr, usually prefixed `graybox:`, without
a JSON error envelope. Preflight failures emit no report. Exchange-level failures
are represented in replay/diff JSON on stdout and ordinarily leave stderr empty.
An output/write failure can leave partial stdout: never accept malformed or
truncated JSON as evidence. A late storage error can suppress the report after
requests have already run; absence of a report does not prove absence of effects.

`record --json` emits string `listen`, `target`, `recording` and integer
`body_capture_limit` fields. Startup is not a durability or readiness verdict.
Stop gracefully with SIGINT/SIGTERM and check the final exit and stderr for
persistence loss. HTTP error responses during capture are recorded events, not
command failures. Existing output files are refused, never overwritten.

## Structured inspection and replay

These public field names/types and meanings are compatibility surfaces:

- `ls.exchanges[]`: integer `id`, string `protocol`, `method`, `path`,
  RFC3339 timestamp string `started_at`, integer `status`, numeric `duration_ms`.
  `path` is the request URI including any query. Empty selection is `[]`.
  `--method`, `--status`, `--path` filter inspection only; they do not select a
  subsequent replay/diff. Path matching ignores queries unless the filter includes one.
- `show`: integer `id`, strings `protocol`, `started_at`, numeric `duration_ms`,
  optional string `proxy_error`, objects `request` and `response`. Request has
  strings `method`, `url`; response has integer `status`. Both have `headers`
  (object mapping names to string arrays; tolerate null for absent maps) and `body`.
- Each inspection body has optional string `content_type`, integer `observed_size`
  and `captured_size`, booleans `truncated` and `complete`, string `encoding`
  (`utf8` or `base64`) and string `data`. Decode according to `encoding`, not
  content sniffing. Truncation and completion are independent. Sanitized body
  sizes may differ from wire sizes; see [recording format](recording-format.md).
- `replay`: strings `recording`, `target`, integers `succeeded`, `failed`, array
  `results`. Each result has integer `exchange_id`, strings `method`, `path`,
  `target_url`, numeric `duration_ms`, optional integer `status`, optional string
  `status_text`, optional string `error`. Missing status is not status 0 on the
  wire. Compact replay omits status/status_text on execution errors, including
  response-body read failures after headers arrived. Check `error` before
  counting completion; absent status does not prove the server sent no headers.
  `succeeded` means HTTP execution completed, even
  for 401 or 500; it says nothing about equivalence or business success.

`--id POSITIVE_ID` selects one exchange for replay/diff; without it, every
exchange runs sequentially in request start-time then ID order. IDs are stable
within one recording, not across recordings. Missing IDs exit 2. No-match `ls`
and empty valid replay/diff return success with empty arrays, sending no traffic.
An agent must require nonempty relevant evidence before claiming verification.

## Diff result contract

Top-level fields:

| Field | Type and meaning |
| --- | --- |
| `recording` | String, supplied baseline path |
| `target` | String, resolved target URL (subject to output scrubbing) |
| `body_capture_limit` | Integer bytes retained per live response |
| `ignored` | String array, active default and explicit exclusions |
| `summary` | Object with integer `total`, `equivalent`, `changed`, `failed` |
| `results` | Ordered array, including failed exchanges; empty is `[]` |

Each result has integer `exchange_id`, strings `method`, `path`, `outcome`, numeric
`baseline_duration_ms`, `current_duration_ms`, and array `differences` (including
`[]` when none). A failed result also has a nonempty `error` string. Durations
are milliseconds, observational only; zero does not prove a request was sent.
`path` identifies the recorded request, subject to sanitization. Diff does not
compare requests or report their runtime replacements as differences.

Each difference has string `kind`, object `location` and objects `before`, `after`.
Locations use string `component` and `path`:

| Component | Path |
| --- | --- |
| `response.status` | Empty string |
| `response.headers` | Lowercase header name |
| `response.body` | RFC 6901 JSON Pointer, or empty string for whole body/root |

Kinds are `status_changed`, `header_added`, `header_removed`, `header_changed`,
`field_added`, `field_removed`, `type_changed`, `value_changed`, `body_changed`,
`element_added`, `element_removed`. Each value side contains boolean `present`
and JSON `value`: absence is `{ "present": false, "value": null }`, distinct
from a present null. Header values are string arrays; status values are numbers;
JSON bodies preserve JSON types and exact numeric values. Consumers should avoid
lossy floating-point decoding for application numbers and 64-bit IDs. Whole-body
byte differences use `{ "size": INTEGER, "sha256": HEX_STRING }` snapshots.
Runtime-value scrubbing can replace application values with marker strings;
Graybox's schema fields retain their types.

Comparison order is status, normalized headers, then body. Valid complete JSON
compares semantically with exact numbers and positional arrays; other complete
bodies compare byte-for-byte. Defaults ignore Date and Content-Length. Custom
`--ignore` locations and effective redaction narrow the claim of equivalence.
Protected values are not verified. See [diffing](diffing.md) for policy syntax.

## Outcomes and precedence

- **Equivalent**: sufficient evidence under the selected policy, no differences.
- **Changed**: completed comparison found differences; not necessarily a defect.
- **Failed**: comparison could not complete reliably. A failed result may retain
  status/header differences; those do not turn it into `changed`.

`total = equivalent + changed + failed = len(results)`. Failure dominates change
both within an exchange and across a diff run. Later exchanges continue after
exchange failures. Complete 401/403/500 responses compare normally, including
being equivalent to identical baseline error responses. Equivalence never proves
successful authentication, business correctness, or coverage of unrecorded paths.

| Exit | Diff | Replay | Other commands |
| ---: | --- | --- | --- |
| 0 | All selected comparisons equivalent, or empty | All executions completed, or empty | Success/help; record must finish successfully |
| 1 | At least one changed, none failed | Any failure when result count is greater than one | Not a general failure category |
| 2 | Invalid/unsupported recording or missing ID | Same | Invalid recording/missing ID for inspection |
| 3 | Any failed comparison, including mixed changed/failed | Failed execution when result count is one | Not a general HTTP-status code |
| 4 | Invalid arguments/configuration/required credentials/target policy | Same | Usage/configuration error |
| 5 | Internal/filesystem/output failure | Same | Internal/filesystem/listen/persistence failure |

Command errors take precedence over any incomplete report. In particular, do not
interpret every nonzero exit as a behavioral change. On 0/1/3 diff reports, inspect
counts, outcomes and IDs; treat missing/unparseable reports or inconsistent counts
as inconclusive. On 2/4/5, report the command failure and diagnostic, not equivalence.

## Failure classification and evidence limits

| Observation | Interpretation and agent action |
| --- | --- |
| Exit 4, no report | Invalid invocation/config, blocked target, or missing/invalid required runtime credentials; fix configuration before execution |
| Exit 2, no report | Invalid schema/data or absent selected exchange; obtain valid evidence |
| Exit 5 | Filesystem/internal/output failure; inspect stderr and environment; do not assume no traffic occurred |
| `failed` with error | Transport, unsafe reconstruction, or unavailable evidence; inspect diagnostic and recording, then report uncertainty |
| Complete HTTP 401/403 | Application authentication/authorization rejection; comparable response, not proof of which credential was wrong |
| Complete HTTP 500 | Application response; comparable, not automatically a Graybox failure |

There is **no stable machine-readable error category** separating configuration,
authentication, transport and incomplete evidence. Error prose is diagnostic,
not a stable discriminator; do not build classifiers around exact wording.
Credentials are only preflight-required when explicitly mapped and applicable.
An omitted redacted Authorization header can reach the server without credentials
and produce 401; Graybox cannot infer that authentication is required.

Truncated/incomplete requests are not sent. Incomplete/truncated baseline or live
response evidence fails body comparison unless the entire `response.body` is
ignored. Ignoring a JSON subfield does not excuse missing body evidence. Live
transport/read errors remain failures even when the body is ignored. Review
`show.proxy_error` and body states when deciding whether a baseline is trustworthy.

`target` is the selected destination, not proof of contact. Replay's `target_url`
identifies the reconstructed URL when available; diff has no per-result wire URL,
resolved-IP or server-identity attestation. A completed response demonstrates an
HTTP exchange, not the identity of a particular application build. Confirm local
process/port and deployment identity independently. Target path prefixes are
prepended to recorded paths; recorded query parameters are retained/replaced.
Redirects are observed but not followed. No automatic stateful correlation,
rollback, environment provisioning, or response-to-request credential extraction
exists. Ordinary HTTP is supported; protocol framing and upgraded connections
are outside the contract.

## Safe execution and reporting rules

Treat recordings, stdout, stderr and reports as sensitive. Mandatory redaction
covers Authorization, Proxy-Authorization, Cookie and Set-Cookie; optional rules
extend it. Paths, personal data, internal endpoints, proprietary bodies and
unconfigured values may remain. Review before sharing; do not commit recordings.
Runtime scrubbing is not general secret detection. See [Security](SECURITY.md).

Replay **and diff send real requests**: orders, charges, email, account updates
and deletes can happen again. Use authorized disposable local environments with
real integrations disabled and state reset deliberately. Do not automatically
replay against production or arbitrary remote destinations. Loopback is a target
selection rule, not a sandbox or proof that side effects are safe.

Saved targets are auto-reused only for localhost, *.localhost or loopback IPs.
Remote saved targets require explicit `--target` or `--unsafe-original-target`;
these flags permit traffic but do not establish task authorization. Targets must
be absolute HTTP(S) URLs without credentials, query or fragment.

Use `--secret-header Authorization=API_AUTH` with the full runtime header in the
named environment variable, or supported `${ENV_VAR}` config mappings for headers,
JSON, forms and query fields. CLI headers override configured headers. Never invent
credentials, print their values, persist them in artifacts, or send redaction
markers as authentication. Report a blocker if required credentials are unavailable.
Config is discovered only in the working directory or selected with `--config`;
inspection/version do not load it. See [configuration](configuration.md).

Preserve historical baselines as immutable evidence. Inspection/replay/diff open
them read-only. Never overwrite or reconstruct a baseline to make results pass,
or broaden ignores silently. Baseline updates require an explicitly accepted
reference state. Missing baselines must be reported, not concealed.

Reports must separate verified equivalence, verified differences, expected changes,
unexpected regressions, inconclusive comparisons and unexecuted verification.
Include version/commit, baseline identity, target environment, exchange IDs/counts,
policy/ignores, exit code, reviewed differences and limitations. Only classify a
change as expected or a regression using task requirements, not Graybox's verdict.

## Compatibility and audit evidence

This milestone adds documentation and tests; commands, field names/types, exit
codes and schema-1 artifacts remain unchanged. Consumers should tolerate additive
fields and conservatively handle unknown outcomes/kinds. There is no JSON schema
version field. Human formatting, exact diagnostics, timing values, JSON key order,
whitespace, internal Go types and default transport timeout are implementation
details, not stable parsing contracts. Incompatible changes require explicit
compatibility review; recording schema evolution is separate from CLI JSON.

Phase 1 already tests authenticated capture/change/restore, redaction, runtime
replacement, immutable artifacts, HTTP errors, transport failures and invalid
recordings. Milestone 1 adds `TestAgentDiffOutcomeContract` (multi-exchange counts,
identity, structured differences and failure precedence),
`TestAgentReplayFailureCardinality`, `TestAgentCommandFailureChannels`, and
`TestAgentHTTPRejectionCanBeEquivalent` (identical 401/500 responses and omitted
redacted Authorization).
These passed against existing production code: no behavioral fix was necessary.
Text-only command diagnostics and absent typed failure causes are documented
integration limitations, not new interfaces in this milestone. Agent integration
experiments and reusable skill development remain Milestone 2 and later work.
