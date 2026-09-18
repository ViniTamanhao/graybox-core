# Architecture

Graybox V0 is a single CLI process with no daemon or hosted component.

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
- `internal/sanitize` owns the small, deterministic V0 header-redaction policy.
- `internal/storage` owns schema creation, validation, transactional writes, and reads from a `.graybox` SQLite database.
- `internal/replay` reconstructs requests, replaces the target, removes unsafe transport headers and redacted credentials, and executes requests sequentially with a dedicated transport. It disables implicit compression behavior, does not follow redirects, and refuses truncated or incomplete request bodies.
- `internal/cli` parses commands and renders domain results for humans or as deliberate JSON contracts.
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

## Shutdown and ownership

The `record` command owns one HTTP server and one writable storage connection. On `SIGINT` or `SIGTERM`, it calls `http.Server.Shutdown` with a bounded grace period. Handler completion includes the SQLite transaction, so successful shutdown waits for in-flight records before closing the database. Each exchange write is one transaction. A failed exchange transaction does not interrupt proxy traffic, but it increments a persistence-failure counter; shutdown prints the number of lost exchanges and returns a non-zero exit status. Upstream HTTP responses and transport failures are recorded application events, not persistence failures.

`ls`, `show`, and replay source access open recordings with a platform-correct SQLite `file:` URI in `mode=ro`. Their connections reject writes, and read-only close does not run maintenance writes such as `PRAGMA optimize`. New recording files use exclusive creation so existing files are not overwritten and request restrictive `0600` permissions where the operating system supports POSIX modes.

V0 records ordinary HTTP application exchanges and does not promise HTTP/2 framing fidelity, server push, upgraded connections, WebSockets, or protocol-specific gRPC behavior.
