# Architectural guidance for agents

See [the detailed architecture](../docs/architecture.md) for data flow and lifecycle.
Respect existing ownership instead of creating parallel engines:

| Package | Owns |
| --- | --- |
| cmd/graybox | Signals, injected build version, process exit |
| internal/cli | Argument/config orchestration, credential resolution, target policy, human/JSON rendering and output scrubbing |
| internal/capture | Reverse proxy, bounded capture, transport and persistence diagnostics |
| internal/recording | Exchange/request/response/body state domain model |
| internal/storage | SQLite schema validation, transactions and read-only access |
| internal/replay | Request reconstruction, explicit replacements, sequential HTTP execution |
| internal/diff | Deterministic response comparison, differences and outcomes |
| internal/sanitize | Mandatory/configured redaction, field transformations and historical marker inference |
| internal/config | Strict YAML parsing without resolving runtime credentials |

Capture forwards traffic, sanitizes retained data and commits an exchange.
Replay reads a recording without writes, reconstructs requests and supplies
execution-only credentials. Diff applies a consistent protection policy to
baseline/live responses before comparison; renderers expose the same outcomes.

Do not put HTTP/storage work in renderers or presentation rules in domain types.
Keep secrets out of metadata and parsed configuration. Preserve schema 1 and its
independent body truncation/completion states. Missing evidence means `failed`,
not equivalent; HTTP application errors still have comparable responses. Preserve
ordering, exact JSON number semantics and machine-readable field types. Changes
to these boundaries need concrete behavior tests and contract documentation.
