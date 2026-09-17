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

The proxy receives ordinary HTTP requests on an explicit local address. It forwards each request with Go's `net/http` reverse-proxy machinery, captures the observed response, redacts selected headers, and commits the completed exchange to SQLite. Individual bodies are buffered pragmatically in V0; sessions are not held in memory.

## Responsibilities

- `internal/recording` defines the exchange, request, response, timing, filter, and summary domain values. It contains no storage or CLI behavior.
- `internal/capture` owns reverse proxying and converts observed HTTP traffic into completed domain exchanges.
- `internal/sanitize` owns the small, deterministic V0 header-redaction policy.
- `internal/storage` owns schema creation, validation, transactional writes, and reads from a `.graybox` SQLite database.
- `internal/replay` reconstructs requests, replaces the target, removes unsafe transport headers and redacted credentials, and executes requests sequentially.
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

The `record` command owns one HTTP server and one storage connection. On `SIGINT` or `SIGTERM`, it calls `http.Server.Shutdown` with a bounded grace period. Handler completion includes the SQLite transaction, so successful shutdown waits for in-flight records before closing the database. Each exchange write is one transaction.
