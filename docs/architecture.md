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

The proxy receives ordinary HTTP requests on an explicit address that defaults to `127.0.0.1:9000`. It forwards each request with Go's `net/http` reverse-proxy rewrite API, sets the upstream `Host` to the configured target, removes inbound forwarding headers, and explicitly constructs `X-Forwarded-For`, `X-Forwarded-Host`, and `X-Forwarded-Proto` from the received connection. It captures the observed response, redacts selected headers, and commits the completed exchange to SQLite.

Request and response bodies stream through the proxy. A bounded prefix is retained for recording (10 MiB per body by default), while counters preserve the observed original size and mark truncation explicitly. A transport failure produces a client-facing 502 and a non-empty recorded proxy error; an upstream 502 has no proxy error.

## Responsibilities

- `internal/recording` defines the exchange, request, response, timing, filter, and summary domain values. It contains no storage or CLI behavior.
- `internal/capture` owns reverse proxying and converts observed HTTP traffic into completed domain exchanges.
- `internal/sanitize` owns the small, deterministic V0 header-redaction policy.
- `internal/storage` owns schema creation, validation, transactional writes, and reads from a `.graybox` SQLite database.
- `internal/replay` reconstructs requests, replaces the target, removes unsafe transport headers and redacted credentials, and executes requests sequentially with a dedicated transport. It disables implicit compression behavior, does not follow redirects, and refuses truncated request bodies.
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

The `record` command owns one HTTP server and one writable storage connection. On `SIGINT` or `SIGTERM`, it calls `http.Server.Shutdown` with a bounded grace period. Handler completion includes the SQLite transaction, so successful shutdown waits for in-flight records before closing the database. Each exchange write is one transaction. `ls`, `show`, and replay source access open recordings in SQLite read-only mode; read-only close does not run maintenance writes such as `PRAGMA optimize`.

V0 records ordinary HTTP application exchanges and does not promise HTTP/2 framing fidelity, server push, upgraded connections, WebSockets, or protocol-specific gRPC behavior.
