# Contributing to Graybox

Graybox is intentionally a small API recorder and debugger. Focused issues and pull requests are welcome, especially for HTTP correctness, cross-platform behavior, tests, documentation, and the recording format.

## Development setup

```bash
git clone https://github.com/opemori/graybox-core.git
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
- `internal/cli`: parsing and human/JSON rendering

Keep changes within Graybox's recorder/debugger scope. Prefer explicit standard-library code and small tests over speculative abstractions. If a change alters persisted data or JSON output, update the relevant documentation and add compatibility-focused tests.

Before opening a pull request, run `gofmt` on Go files and make sure all three validation commands above pass. Describe the behavior being changed and how it was tested.
