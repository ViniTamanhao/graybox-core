# Graybox recording format

This document describes early format version 1, written by Graybox V0. A `.graybox` file is an ordinary SQLite 3 database. The V0 format is documented for interoperability, but it is not yet promised stable forever.

## Version and metadata

The `metadata` table is a string key/value map. Every valid recording contains:

| Key | Meaning |
| --- | --- |
| `format` | literal `graybox` |
| `schema_version` | decimal schema version; V0 writes `1` |
| `created_at` | UTC RFC 3339 timestamp with nanosecond precision |
| `graybox_version` | CLI version that created the file |
| `target_url` | original upstream target, added by `graybox record` |
| `body_capture_limit` | maximum retained bytes per request or response body, added by `graybox record` |

Readers reject missing Graybox metadata and unsupported schema versions. Future versions can migrate based on `schema_version`; V0 deliberately has no larger migration framework.

## Schema

```sql
CREATE TABLE metadata (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE exchanges (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    protocol        TEXT NOT NULL,
    started_at      TEXT NOT NULL,
    completed_at    TEXT NOT NULL,
    duration_ns     INTEGER NOT NULL CHECK (duration_ns >= 0),
    request_method  TEXT NOT NULL,
    request_url     TEXT NOT NULL,
    response_status INTEGER NOT NULL,
    proxy_error     TEXT NOT NULL
);

CREATE TABLE request_headers (
    exchange_id INTEGER NOT NULL REFERENCES exchanges(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    value       TEXT NOT NULL,
    ordinal     INTEGER NOT NULL,
    PRIMARY KEY (exchange_id, name, ordinal)
);

CREATE TABLE response_headers (
    exchange_id INTEGER NOT NULL REFERENCES exchanges(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    value       TEXT NOT NULL,
    ordinal     INTEGER NOT NULL,
    PRIMARY KEY (exchange_id, name, ordinal)
);

CREATE TABLE request_bodies (
    exchange_id INTEGER PRIMARY KEY REFERENCES exchanges(id) ON DELETE CASCADE,
    content       BLOB NOT NULL,
    original_size INTEGER NOT NULL CHECK (original_size >= 0),
    captured_size INTEGER NOT NULL CHECK (captured_size >= 0),
    truncated     INTEGER NOT NULL CHECK (truncated IN (0, 1)),
    CHECK (captured_size = length(content)),
    CHECK (captured_size <= original_size),
    CHECK (truncated = 1 OR captured_size = original_size)
);

CREATE TABLE response_bodies (
    exchange_id INTEGER PRIMARY KEY REFERENCES exchanges(id) ON DELETE CASCADE,
    content       BLOB NOT NULL,
    original_size INTEGER NOT NULL CHECK (original_size >= 0),
    captured_size INTEGER NOT NULL CHECK (captured_size >= 0),
    truncated     INTEGER NOT NULL CHECK (truncated IN (0, 1)),
    CHECK (captured_size = length(content)),
    CHECK (captured_size <= original_size),
    CHECK (truncated = 1 OR captured_size = original_size)
);
```

The actual database also has indexes on exchange start time, request method, and response status.

## Representation

- `exchanges.id` is the stable numeric exchange ID used for lookup. SQLite assigns IDs in capture completion order; list output is ordered by `started_at` and then ID.
- `protocol` is `http` in V0. HTTP version-specific framing, including HTTP/2 framing, is not part of the recorded domain model.
- `request_url` is the origin-form request URI: escaped path plus query string. The replay target supplies scheme and authority.
- `started_at` and `completed_at` are UTC RFC 3339 timestamps with nanosecond precision.
- `duration_ns` is total proxy-observed duration in nanoseconds.
- Header names and values are stored as SQLite text. Each value occupies a row; `ordinal` preserves repeated-value order for a given name.
- `proxy_error` is empty for an upstream response, including an upstream 502. It contains the Go proxy/transport error when Graybox itself generated the recorded 502.
- Body `content` is the captured byte prefix in a SQLite BLOB, including empty and non-UTF-8 bodies. `original_size` is the observed byte count, or the larger declared request `Content-Length` when the upstream did not consume the complete request. `captured_size` is the retained BLOB length, and `truncated` explicitly states that bytes were omitted. Media type remains in the corresponding `Content-Type` header.
- One exchange insert, all of its headers, and both bodies are committed in one transaction.

The capture limit bounds memory use per in-flight body without truncating traffic sent through the proxy. `show` exposes all size and truncation fields. Replay refuses a truncated request body because sending its prefix would not reproduce the recorded request.

## Redaction

Before persistence, V0 replaces every value of these request or response headers with the literal `<REDACTED>`:

- `Authorization`
- `Proxy-Authorization`
- `Cookie`
- `Set-Cookie`

Matching is case-insensitive. The number of repeated values remains visible. V0 does not sanitize bodies, URLs, arbitrary headers, or application-specific secrets. A recording must therefore be treated as sensitive even after automatic redaction.

Replay omits a complete header if any of its recorded values is `<REDACTED>`; it never transmits that marker as a credential.

## Compatibility expectations

Format readers must check `format` and `schema_version` before reading exchange data. Schema version 1 readers should also validate the required columns: the format was corrected in place before the public V0.1.0 release and incompatible pre-release schema-1 prototypes are rejected as invalid. New compatible metadata keys may be added without changing the schema version; table or representation changes after release require a future migration/version decision.
