# Behavioral diffing

`graybox diff` replays recorded requests and compares the current HTTP responses with the responses stored in a `.graybox` recording. It turns a recording into a behavioral baseline for the workflow:

```text
record
→ inspect
→ fix
→ diff
→ verify
```

Run all recorded exchanges sequentially:

```bash
graybox diff bug.graybox
```

Run one exchange by its stable recording ID:

```bash
graybox diff bug.graybox --id 42
```

Diffing does not change the recording. It observes the current response and reports whether each selected exchange is `equivalent`, `changed`, or `failed`.

## What is compared

Comparison proceeds deterministically in this order:

1. response HTTP status
2. response headers
3. response body

Status codes are compared numerically. Header names are case-insensitive; repeated header values retain their recorded order. Header additions, removals, and changed values are separate structured differences.

Durations are observations only. Baseline and current durations are included in JSON output, but latency does not affect equivalence.

### JSON bodies

If both relevant response bodies are complete and valid JSON, Graybox compares their decoded JSON values semantically:

- Object key order and whitespace or formatting do not matter.
- Object fields are compared by name.
- Missing values are distinct from explicit JSON `null`.
- Arrays are positional; element order matters.
- JSON numbers are compared exactly by numeric value without conversion through `float64`.
- Equivalent number spellings such as `1`, `1.0`, and `1e0` compare equal.

Object fields and array positions are reported as RFC 6901 JSON Pointers, such as `response.body#/users/0/name`.

### Non-JSON bodies

If either complete body is not valid JSON, the bodies are compared byte-for-byte. Media type does not weaken this rule. When they differ, human and JSON reports identify the whole body and use a compact snapshot instead of copying the body into the report:

```json
{
  "size": 123,
  "sha256": "hexadecimal digest"
}
```

The comparison still examines every retained byte; the size and SHA-256 value are the report representation.

### Incomplete or truncated bodies

A truncated capture or an incomplete response stream does not contain enough evidence to establish body equivalence. The exchange is therefore `failed`, even if its retained prefix matches, unless `response.body` is ignored.

Ignoring `response.body` skips body comparison and body availability checks entirely. Status and non-ignored headers are still compared. Recorded request bodies follow replay safety rules: a request body that was truncated or incomplete is not sent.

## Outcomes

Each selected exchange has exactly one outcome:

- `equivalent`: comparison completed and no behavioral differences were found.
- `changed`: comparison completed and one or more differences were found.
- `failed`: comparison could not be completed.

`changed` and `failed` are deliberately distinct. A transport error, an unsafe or incomplete request, or unavailable response-body evidence is not reported as a behavioral change. A failed exchange does not stop later exchanges from being processed. A failed result can retain status or header differences found before body comparison failed.

## Ignored locations

Graybox ignores these response headers by default because servers commonly regenerate them:

```text
Date
Content-Length
```

Default and user-provided header locations are displayed in normalized lowercase form. Add exclusions with repeatable `--ignore` flags:

```bash
graybox diff bug.graybox \
  --ignore 'response.body#/metadata/request_id'
```

Supported locations are:

```text
response.status
response.headers
response.headers.NAME
response.body
response.body#/JSON/POINTER
```

`response.headers` ignores every response header. `response.headers.NAME` ignores one case-insensitive header name. `response.body` ignores the complete body. A body pointer ignores that JSON value and all of its descendants.

Body paths use RFC 6901 JSON Pointer syntax. Each path begins with `/`; array positions use decimal indices. Within a pointer token, encode `~` as `~0` and `/` as `~1`. For example, the JSON key `a/b` is addressed as:

```bash
graybox diff bug.graybox --ignore 'response.body#/a~1b'
```

JSON Pointer is not JSONPath: expressions such as `$.metadata.request_id` and wildcard selectors are not supported.

## Selecting an exchange

`--id ID` compares only the exchange with that positive numeric ID:

```bash
graybox diff bug.graybox --id 42
```

A recording without that exchange exits with code `2`. Without `--id`, Graybox processes every exchange sequentially in recording list order.

## Target safety

Choose a replacement target explicitly when needed:

```bash
graybox diff bug.graybox \
  --target http://localhost:8081
```

Target selection follows replay policy:

- An explicit `--target` is used when supplied.
- Without `--target`, the saved target is reused automatically only for `localhost`, `*.localhost`, and loopback IP addresses.
- A saved remote target requires an explicit `--target` or `--unsafe-original-target`.
- Redirects are returned as the observed response and are not followed.
- Redacted credentials and hop-by-hop headers are not replayed.
- Truncated or incomplete recorded request bodies are not replayed.

Both `diff` and `replay` can send recorded payloads to a server. Review the recording and destination before running either command against a non-local target.

## Response capture limit

`graybox record` writes the configured `body_capture_limit` into recording metadata. Diff uses that value as the bounded prefix retained from each replayed response, so baseline and current evidence use the same capture limit. The live response is still drained after the limit is reached.

Schema-1 recordings created before this optional metadata was written remain usable. If `body_capture_limit` is absent, diff uses the default replay response capture limit. An invalid non-positive metadata value makes the recording invalid rather than silently selecting another limit.

## Human output

Human output shows the selected target, active ignores, each exchange outcome, structured differences, any failure, and aggregate counts. For example:

```text
Diffing 2 exchanges against http://localhost:8080
Ignoring: response.headers.date, response.headers.content-length

1 GET /hello
  equivalent

2 POST /echo
  changed
  response.body#/message
    "old" -> "new"

1 equivalent
1 changed
0 failed
```

Large values may be abbreviated in human output. Machine JSON retains the structured difference values; changed non-JSON bodies remain represented by their size and SHA-256 snapshots.

## JSON output

Use `--json` for scripts, CI, and coding agents:

```bash
graybox diff bug.graybox --json
```

The top-level object contains:

- `recording`: recording path passed to the command.
- `target`: resolved target URL.
- `body_capture_limit`: replayed response capture limit in bytes.
- `ignored`: active default and user-provided ignored locations.
- `summary`: `total`, `equivalent`, `changed`, and `failed` counts.
- `results`: ordered per-exchange results.

Each result contains `exchange_id`, `method`, `path`, `outcome`, `baseline_duration_ms`, `current_duration_ms`, and `differences`. Failed results also contain an `error` string. Each structured difference has a `kind`, a `location` object with `component` and `path`, and `before` and `after` values.

Presence is explicit so a missing JSON field or array element remains distinct from JSON `null`:

```json
{
  "present": false,
  "value": null
}
```

versus:

```json
{
  "present": true,
  "value": null
}
```

Changed exact bodies use the size and SHA-256 snapshot shown above. Durations are numeric milliseconds and remain observational.

## Exit codes

| Code | Meaning |
| ---: | --- |
| 0 | all selected exchanges are equivalent |
| 1 | one or more behavioral changes were found |
| 2 | invalid or unsupported recording, or missing exchange |
| 3 | one or more comparisons could not complete |
| 4 | invalid CLI arguments or configuration |
| 5 | internal or filesystem error |

If a run contains both changed and failed exchanges, code `3` takes precedence. Replay retains its own command-specific meaning for exit code `1`.
