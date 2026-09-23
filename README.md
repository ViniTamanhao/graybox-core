# Graybox

A local-first API flight recorder and behavioral debugger for developers and AI coding agents.

Graybox captures HTTP traffic into portable `.graybox` recordings that can be inspected, replayed, and compared against current application behavior without a cloud service, daemon, account, or privileged network access.

**Record what happened. Reproduce it. Understand it.**

> **Status:** Graybox V1 (`v0.2.0`) adds semantic response diffing. Recording schema 1 remains current and compatible with V0 recordings.

```console
$ graybox record --target http://localhost:8080 --output bug.graybox
Graybox recording

Listening:   http://127.0.0.1:9000
Forwarding:  http://localhost:8080
Recording:   bug.graybox
Body limit:  10485760 bytes

# In another terminal:
$ curl http://127.0.0.1:9000/hello
{"message":"hello from Graybox"}

# After stopping the recorder with Ctrl+C:
$ graybox ls bug.graybox
ID   METHOD   PATH     STATUS   TIME
1    GET      /hello  200      1ms
```

## Why Graybox?

An API failure is easier to debug when the effective upstream request, observed response, headers, captured body bytes, and timing survive after the process stops. Graybox creates a durable recording of that behavior. Body capture is bounded: traffic continues to stream after the configured limit, while the recording keeps the captured prefix plus `observed_size`, `captured_size`, `truncated`, and `complete` metadata. A recording is an ordinary SQLite database, so it works with the Graybox CLI, `sqlite3`, scripts, CI jobs, and coding agents.

Graybox is a focused local development tool—not an APM, packet analyzer, production monitoring system, service mesh, or hosted API client.

## Install

Graybox requires Go 1.26.0 or newer to build from source.

```bash
go install github.com/ViniTamanhao/graybox-core/cmd/graybox@latest
```

For a repository checkout:

```bash
go build -o graybox ./cmd/graybox
./graybox version
```

Release builds can inject a version:

```bash
go build -ldflags "-X main.version=v0.2.0" -o graybox ./cmd/graybox
```

## Quick start

Run the included example API:

```bash
go run ./examples/server
```

In a second terminal, start the explicit reverse proxy:

```bash
go run ./cmd/graybox record \
  --target http://localhost:8080 \
  --output example.graybox
```

Send traffic through Graybox:

```bash
curl http://127.0.0.1:9000/hello
curl -X POST http://127.0.0.1:9000/echo \
  -H 'Content-Type: application/json' \
  -d '{"from":"graybox"}'
```

Press Ctrl+C in the recorder terminal. Graybox stops accepting connections, lets in-flight handlers finish, and closes the SQLite recording cleanly.

If one or more observed exchanges cannot be committed to SQLite, proxying continues where possible, but shutdown reports the number lost and exits non-zero. Upstream HTTP error responses and connection failures are valid recorded events. When response streaming is interrupted after it starts, Graybox records the partial exchange when possible and marks the response body incomplete.

## List recordings

```bash
graybox ls example.graybox
graybox ls example.graybox --status 500
graybox ls example.graybox --method POST
graybox ls example.graybox --path /echo
```

Results are ordered by request start time and then by stable exchange ID. Method matching is case-insensitive. A path filter matches the exact escaped URL path while ignoring the query, so `/users/a%2Fb` remains distinct from `/users/a/b`; if the filter contains `?`, it matches the complete request URI exactly.

## Inspect an exchange

```bash
graybox show example.graybox 2
```

JSON and text bodies are rendered as text, with valid JSON indented. Binary bodies are never written raw to a terminal; the human view reports their size and media type. Capture truncation and an interrupted body stream are labeled independently.

## Replay requests

Replay every request sequentially against a loopback target saved in the recording:

```bash
graybox replay example.graybox
```

Or select an exchange and replace the original target:

```bash
graybox replay example.graybox \
  --id 2 \
  --target http://localhost:8081
```

Replay preserves the method, escaped path, raw query representation, request body when fully captured and complete, and ordinary effective outbound request headers. Graybox deliberately preserves unusual raw queries—including semicolons and percent encoding—for debugging fidelity. A path prefix in `--target` is prepended, and the target supplies the replay `Host`. Graybox omits content length, connection, transfer-encoding, other hop-by-hop headers, and any header with a redacted value. `graybox replay` reports the observed response without judging equivalence; use `graybox diff` to compare it with the recorded response.

Graybox follows no redirects during replay: a redirect is the result for that exchange. For safety, an omitted `--target` uses the recorded target only when its host is `localhost`, a `*.localhost` name, or a loopback IP address. To contact any other original target, provide an explicit `--target` or acknowledge the risk with `--unsafe-original-target`. A request whose recorded body was truncated or incomplete is not replayed.

## Compare behavior

Replay every recorded request and compare the current responses with the recorded baseline:

```bash
graybox diff example.graybox
```

Replace the saved target when comparing another local instance:

```bash
graybox diff example.graybox \
  --target http://localhost:8081
```

Select one exchange by its stable ID:

```bash
graybox diff example.graybox --id 2
```

Ignore a volatile JSON location with an RFC 6901 JSON Pointer:

```bash
graybox diff example.graybox \
  --ignore 'response.body#/metadata/request_id'
```

Diff compares response status, headers, and body. Complete valid JSON bodies are compared semantically: formatting and object key order do not matter, and numeric values are compared exactly. Other complete bodies are compared byte-for-byte. `Date` and `Content-Length` response headers are ignored by default.

Each selected exchange is `equivalent`, `changed`, or `failed`. A failed comparison does not stop later exchanges from being processed. See [Behavioral diffing](docs/diffing.md) for comparison rules, ignore syntax, target safety, output contracts, and exit codes.

## JSON for scripts and agents

`record`, `ls`, `show`, `replay`, `diff`, and `version` accept `--json`. Structured data goes to stdout; diagnostics go to stderr. Human and JSON rendering consume the same domain results.

```bash
graybox ls example.graybox --json | jq '.exchanges[] | select(.status >= 500)'
graybox show example.graybox 2 --json | jq '.request.body'
graybox replay example.graybox --id 2 --json
graybox diff example.graybox --json | jq '.results[] | select(.outcome == "changed")'
```

Bodies have an explicit contract:

```json
{
  "content_type": "application/octet-stream",
  "observed_size": 4,
  "captured_size": 4,
  "truncated": false,
  "complete": true,
  "encoding": "base64",
  "data": "AAEC/w=="
}
```

Textual bodies use `"encoding": "utf8"`; binary bodies use `"encoding": "base64"`. `observed_size` is the number of bytes Graybox actually saw, while `captured_size` is the retained byte count. `truncated` means the capture limit omitted observed bytes; `complete` means the HTTP body stream finished normally. These states are independent: `truncated: false` does not imply `complete: true`. Durations are numeric milliseconds and timestamps are RFC 3339 with nanosecond precision. Header values are arrays so repeated fields are preserved. A `proxy_error` field distinguishes a Graybox-generated 502 from an upstream 502.

## Recording format

A `.graybox` file is SQLite with a normalized, versioned schema. Schema 1, introduced by V0, is still used by V1; semantic diffing does not change the recording representation. It stores metadata, exchanges, repeated request/response headers, bounded request/response BLOB captures, body-size metadata, and proxy errors. You can inspect it directly:

```bash
sqlite3 example.graybox '.tables'
sqlite3 example.graybox 'select id, request_method, request_url, response_status from exchanges;'
```

See [the recording format](docs/recording-format.md) for the complete schema and compatibility policy, and [the architecture](docs/architecture.md) for component responsibilities.

## Security

Recordings can contain passwords, API keys, personal data, private identifiers, internal URLs, and application payloads. Graybox automatically replaces values of `Authorization`, `Proxy-Authorization`, `Cookie`, and `Set-Cookie` headers with `<REDACTED>` before persistence. This is intentionally predictable but **not exhaustive**; secrets in bodies, URLs, and other headers are not removed.

Treat every `.graybox` file as potentially sensitive. Graybox never uploads recordings automatically. Read [SECURITY.md](SECURITY.md) before sharing one.

`record` listens on `127.0.0.1:9000` by default. Supplying a wildcard or non-loopback `--listen` address exposes the proxy to other machines and should be a deliberate choice.

## Exit codes

| Code | Meaning |
| ---: | --- |
| 0 | success; for diff, all selected exchanges are equivalent |
| 1 | diff found changes, or a multi-request replay had failures |
| 2 | invalid/unsupported recording, or missing exchange |
| 3 | a single replay failed, or one or more diff comparisons failed |
| 4 | invalid CLI arguments or configuration |
| 5 | internal/filesystem failure, including an incomplete recording caused by persistence failure |

Exit-code meaning is command-specific: replay retains its established meaning for code `1`. For diff, `changed != failed`; code `3` takes precedence over code `1` when a run contains both changed and failed exchanges. An HTTP 4xx or 5xx is still a completed replay, not a transport failure.

## V1 scope and limitations

V1 provides `record`, `ls`, `show`, sequential `replay`, `diff`, `version`, and `help` for ordinary HTTP/1.x application traffic. It uses an explicit reverse proxy. Requests and responses stream through it; by default Graybox retains at most 10 MiB from each body and records observed size, retained size, capture truncation, and stream completion independently. Change this bound with `record --body-limit BYTES`.

Diff compares response status, headers, and body in that order. Complete valid JSON is compared semantically: object order and formatting do not matter, `1`, `1.0`, and `1e0` are equivalent, missing values differ from explicit `null`, and arrays are positional. Other complete bodies are compared byte-for-byte. Durations are reported as observations and do not affect equivalence. Incomplete or truncated response-body evidence cannot establish equivalence unless `response.body` is ignored.

V1 does not provide unordered-array matching, numeric tolerances, regex comparison rules, JSONPath, schema validation, custom comparison scripts, or performance thresholds. Broader non-goals remain: no mock server, configurable sanitizer, query language, daemon, web UI, packet capture, transparent proxy, WebSocket recording, SSE-specific semantics, gRPC decoding, cloud service, tracing, or embedded AI calls.

## Roadmap

- **V0 (`v0.1.0`):** record, list, inspect, replay.
- **V1 (`v0.2.0`):** semantic response diffing.
- **Later:** richer sanitization, mocking, querying, regression suites, CI integrations, gRPC, agent/MCP integrations.

Dates are deliberately not promised; the next feature should follow real recorder/debugger use.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Bug reports, focused fixes, portability improvements, and recording-format feedback are welcome.

## License

Graybox is available under the [MIT License](LICENSE).
