# Contributing to Graybox

Graybox is intentionally a small API recorder and debugger. Focused issues and pull requests are welcome, especially for HTTP correctness, cross-platform behavior, tests, documentation, and the recording format.

Read [AGENTS.md](../AGENTS.md) and the [development guides](../agents/README.md)
before changes. Use Go **1.26.10 or newer** (go.mod is authoritative), TDD by
default, progressive Conventional Commits and incremental documentation.

## Development setup

```bash
git clone https://github.com/ViniTamanhao/graybox-core.git
cd graybox-core
go test ./...
go vet ./...
go build ./...
```

No external server is required; integration tests use `httptest.Server` and temporary SQLite files. For a manual run, use `go run ./examples/server` and follow the README quick start.

## Code map

- `internal/recording`: domain model
- `internal/storage`: SQLite format and persistence
- `internal/capture`: HTTP reverse proxy
- `internal/replay`: request reconstruction and execution
- `internal/sanitize`: persisted-data redaction
- `internal/config`: strict optional YAML parsing
- `internal/cli`: parsing and human/JSON rendering

Keep changes within Graybox's recorder/debugger scope. Prefer explicit standard-library code and small tests over speculative abstractions. If a change alters persisted data or JSON output, update the relevant documentation and add compatibility-focused tests.

Before opening a pull request, run the [full verification commands](../agents/testing.md#verification), including formatting, module consistency, race tests and the configured vulnerability check. Review the complete diff; report actual results and environmental limits. Release changes also require the [packaging smoke checks](installation.md#maintainer-packaging-checks-no-publishing).
