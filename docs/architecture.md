# Architecture

Graybox V1 is a single CLI process with no daemon or hosted component.

```text
client
  |
  v
Graybox reverse proxy -----> recording.graybox
  |
  v
upstream API
```

The proxy receives ordinary HTTP requests on an explicit address that defaults to `127.0.0.1:9000`. It forwards each request with Go's `net/http` reverse-proxy rewrite API, sets the upstream `Host` to the configured target, removes inbound forwarding and hop-by-hop headers, and explicitly constructs `X-Forwarded-For`, `X-Forwarded-Host`, and `X-Forwarded-Proto` from the received connection. Graybox captures the effective outbound header map passed to the HTTP transport, not the untrusted inbound map, then applies redaction before persistence. It deliberately preserves the incoming raw query representation for debugging fidelity. It captures the observed response and commits the exchange to SQLite, including partial responses when streaming aborts after it starts.

Request and response bodies stream through the proxy. A bounded prefix is retained for recording (10 MiB per body by default), while counters preserve the number of bytes Graybox observed. `truncated` records whether the capture limit omitted observed bytes, and `complete` records whether the HTTP body stream finished normally; neither state implies the other. A transport or response-stream failure produces a non-empty recorded proxy error. A Graybox-generated transport response uses status 502, while an upstream 502 has no proxy error.

## Responsibilities

- `internal/recording` defines the exchange, request, response, timing, filter, and summary domain values. It contains no storage or CLI behavior.
- `internal/capture` owns reverse proxying and converts observed HTTP traffic into domain exchanges, including interrupted streams.
- `internal/sanitize` owns the small, deterministic header-redaction policy.
- `internal/storage` owns schema creation, validation, transactional writes, and reads from a `.graybox` SQLite database.
- `internal/replay` reconstructs requests, replaces the target, removes unsafe transport headers and redacted credentials, and executes requests sequentially with a dedicated transport. It disables implicit compression behavior, does not follow redirects, and refuses truncated or incomplete request bodies. Ordinary replay drains response bodies without retaining them; the detailed execution mode used by diff captures a bounded response prefix so the current response can be compared against the recorded baseline.
- `internal/diff` compares each recorded baseline response with the observed replayed response under a rules set of ignored locations and produces deterministic differences and per-exchange outcomes.
- `internal/cli` parses commands, applies the replay/diff target safety policy, parses `--ignore` locations, and renders domain results for humans or as deliberate human/JSON diff DTOs.
- `cmd/graybox` handles process signals, build-time version injection, and exit status.

Human and machine interfaces share the same engine:

```text
                 Graybox engine
                       |
           +-----------+-----------+
           |                       |
           v                       v
    human renderer           JSON renderer
           |                       |
           v                       v
       developer                AI agent
```

## Behavioral verification

`diff` closes the loop from a recording back to current application behavior:

```text
recording.graybox
      |
      v
recorded exchange
      |
      +--------------------+
      |                    |
      |                    v
      |              replay request
      |                    |
      |                    v
      |             observed response
      |                    |
      +---------+----------+
                |
                v
         semantic compare
                |
                v
           diff report
```

Each recorded exchange is loaded, replayed once against the selected target, compared with the observed response, and reported before the next exchange is loaded. A failed exchange does not stop later exchanges. Sequential replay and diff retain approximately one baseline exchange plus one current response at a time instead of retaining the complete recording and all replayed bodies in memory.

## Behavioral comparison

Comparison is deterministic and ordered: response status first, then response
headers in normalized name order, then the response body.

- Status is compared numerically.
- Headers are compared case-insensitively by name, preserving repeated-value
  order. `Date` and `Content-Length` are ignored by default, and `--ignore`
  adds further locations.
- Bodies are compared semantically when both the baseline and the current
  response are complete, valid JSON: object key order and formatting do not
  matter, numbers compare exactly by numeric value (never through `float64`),
  a missing value is distinct from an explicit `null`, and arrays are
  positional. Other bodies are compared byte-for-byte; a changed non-JSON body
  is reported as a size and SHA-256 snapshot rather than a second full copy.
- Truncated or incomplete body evidence cannot establish body equivalence. Such
  an exchange is `failed` unless `response.body` is ignored, which skips body
  comparison and availability checks. A truncated or incomplete recorded
  request body is refused rather than replayed.

Each exchange produces one outcome: `equivalent` (no differences), `changed`
(differences found), or `failed` (comparison could not complete). A failed
exchange does not stop later exchanges. Durations are observations only and
never affect equivalence.

## Shutdown and ownership

The `record` command owns one HTTP server and one writable storage connection. On `SIGINT` or `SIGTERM`, it calls `http.Server.Shutdown` with a bounded grace period. Handler completion includes the SQLite transaction, so successful shutdown waits for in-flight records before closing the database. Each exchange write is one transaction. A failed exchange transaction does not interrupt proxy traffic, but it increments a persistence-failure counter; shutdown prints the number of lost exchanges and returns a non-zero exit status. Upstream HTTP responses and transport failures are recorded application events, not persistence failures.

`ls`, `show`, `replay`, and `diff` open recordings read-only with a platform-correct SQLite `file:` URI in `mode=ro`. Their connections reject writes, and read-only close does not run maintenance writes such as `PRAGMA optimize`. New recording files use exclusive creation so existing files are not overwritten and request restrictive `0600` permissions where the operating system supports POSIX modes.

Graybox records ordinary HTTP application exchanges and does not promise HTTP/2 framing fidelity, server push, upgraded connections, WebSockets, or protocol-specific gRPC behavior.
