# Graybox

A local-first API flight recorder and behavioral debugger for developers and AI coding agents.

Graybox captures HTTP traffic into portable `.graybox` recordings that can be inspected and replayed without a cloud service, daemon, account, or privileged network access.

**Record what happened. Reproduce it. Understand it.**

> **Status:** early V0. The CLI works end to end, while the recording and JSON formats may still evolve before 1.0.

```console
$ graybox record --target http://localhost:8080 --listen :9000 --output bug.graybox
Graybox recording

Listening:   http://localhost:9000
Forwarding:  http://localhost:8080
Recording:   bug.graybox

# In another terminal:
$ curl http://localhost:9000/hello
{"message":"hello from Graybox"}

# After stopping the recorder with Ctrl+C:
$ graybox ls bug.graybox
ID   METHOD   PATH     STATUS   TIME
1    GET      /hello  200      1ms
```

## Why Graybox?

An API failure is easier to debug when the exact request, response, headers, bytes, and timing survive after the process stops. Graybox creates a durable recording of that behavior. A recording is an ordinary SQLite database, so it works with the Graybox CLI, `sqlite3`, scripts, CI jobs, and coding agents.

Graybox is a focused local development tool—not an APM, packet analyzer, production monitoring system, service mesh, or hosted API client.

## Install

Graybox requires Go 1.26 or newer to build from source.

```bash
go install github.com/opemori/graybox-core/cmd/graybox@latest
```

For a repository checkout:

```bash
go build -o graybox ./cmd/graybox
./graybox version
```

Release builds can inject a version:

```bash
go build -ldflags "-X main.version=v0.1.0" -o graybox ./cmd/graybox
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
  --listen :9000 \
  --output example.graybox
```

Send traffic through Graybox:

```bash
curl http://localhost:9000/hello
curl -X POST http://localhost:9000/echo \
  -H 'Content-Type: application/json' \
  -d '{"from":"graybox"}'
```

Press Ctrl+C in the recorder terminal. Graybox stops accepting connections, lets in-flight handlers finish, and closes the SQLite recording cleanly.

## List recordings

```bash
graybox ls example.graybox
graybox ls example.graybox --status 500
graybox ls example.graybox --method POST
graybox ls example.graybox --path /echo
```

Results are ordered by exchange ID. Method matching is case-insensitive. A path filter matches the exact URL path while ignoring the query; if the filter contains `?`, it matches the complete request URI exactly.

## Inspect an exchange

```bash
graybox show example.graybox 2
```

JSON and text bodies are rendered as text, with valid JSON indented. Binary bodies are never written raw to a terminal; the human view reports their size and media type.

## Replay requests

Replay every request sequentially against the target saved in the recording:

```bash
graybox replay example.graybox
```

Or select an exchange and replace the original target:

```bash
graybox replay example.graybox \
  --id 2 \
  --target http://localhost:8081
```

Replay preserves the method, path, query, body, and ordinary request headers. A path prefix in `--target` is prepended. Graybox omits host, content length, connection, transfer-encoding, other hop-by-hop headers, and any header with a redacted value. HTTP response status codes are reported but are not compared with the recording in V0.

## JSON for scripts and agents

`record`, `ls`, `show`, `replay`, and `version` accept `--json`. Structured data goes to stdout; diagnostics go to stderr. Human and JSON rendering consume the same domain results.

```bash
graybox ls example.graybox --json | jq '.exchanges[] | select(.status >= 500)'
graybox show example.graybox 2 --json | jq '.request.body'
graybox replay example.graybox --id 2 --json
```

Bodies have an explicit contract:

```json
{
  "content_type": "application/octet-stream",
  "size": 4,
  "encoding": "base64",
  "data": "AAEC/w=="
}
```

Textual bodies use `"encoding": "utf8"`; binary bodies use `"encoding": "base64"`. Durations are numeric milliseconds and timestamps are RFC 3339 with nanosecond precision. Header values are arrays so repeated fields are preserved.

## Recording format

A `.graybox` file is SQLite with a normalized, versioned schema. V0 stores metadata, exchanges, repeated request/response headers, and raw request/response BLOBs. You can inspect it directly:

```bash
sqlite3 example.graybox '.tables'
sqlite3 example.graybox 'select id, request_method, request_url, response_status from exchanges;'
```

See [the recording format](docs/recording-format.md) for the complete schema and compatibility policy, and [the architecture](docs/architecture.md) for component responsibilities.

## Security

Recordings can contain passwords, API keys, personal data, private identifiers, internal URLs, and application payloads. V0 automatically replaces values of `Authorization`, `Proxy-Authorization`, `Cookie`, and `Set-Cookie` headers with `<REDACTED>` before persistence. This is intentionally predictable but **not exhaustive**; secrets in bodies, URLs, and other headers are not removed.

Treat every `.graybox` file as potentially sensitive. Graybox never uploads recordings automatically. Read [SECURITY.md](SECURITY.md) before sharing one.

## Exit codes

| Code | Meaning |
| ---: | --- |
| 0 | success |
| 1 | a multi-request replay completed with one or more failures |
| 2 | invalid/unsupported recording, or missing exchange |
| 3 | a single replay failed at the network layer |
| 4 | invalid CLI arguments or configuration |
| 5 | internal or filesystem error |

An HTTP 4xx or 5xx is still a completed replay, not a transport failure.

## V0 scope and limitations

V0 provides `record`, `ls`, `show`, and sequential `replay` for ordinary HTTP traffic, plus `version` and `help`. It uses an explicit reverse proxy and buffers each individual request and response body in memory while capturing it. It does not buffer an entire session.

V0 has no semantic diffing, mock server, configurable sanitizer, query language, daemon, web UI, packet capture, transparent proxy, WebSocket recording, SSE-specific behavior, gRPC decoding, cloud service, tracing system, or embedded AI calls.

## Roadmap

- **V0:** record, list, inspect, and replay HTTP exchanges.
- **V1:** semantic response diffing.
- **Later:** richer sanitization, mocking, querying, gRPC awareness, and agent/MCP integrations.

Dates are deliberately not promised; the next feature should follow real recorder/debugger use.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Bug reports, focused fixes, portability improvements, and recording-format feedback are welcome.

## License

Graybox is available under the [MIT License](LICENSE).
