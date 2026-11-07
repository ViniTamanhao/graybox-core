# Testing: RED → GREEN → REFACTOR

TDD is the default for new functionality and reproducible bug fixes.

## RED

Understand the expected behavior and select the right test level. Write a focused
behavior test and run it before implementation. Confirm failure for the intended
reason. A syntax error, missing import or environment failure is not RED. Bug
regressions must reproduce the actual defect, not a hypothetical implementation.

## GREEN

Make the smallest reasonable change that passes the test. Do not introduce
unrelated functionality or abstractions.

## REFACTOR

Improve clarity while keeping tests passing. Avoid architectural speculation.

## Graybox coverage priorities

Use deterministic local httptest servers, temporary SQLite files, synthetic
canaries and channel-based synchronization. Avoid external services, arbitrary
sleeps and fixed ports. Bound waits so failures cannot hang CI. Test observed
behavior and failures, not private implementation shape.

Cover HTTP status/headers/body correctness, capture and transaction persistence,
replay target/redirect/partial-request safety, configured and mandatory redaction,
runtime injection, semantic comparison, JSON types, exit-code precedence and
legacy SQLite compatibility. Security tests must check both renderers, errors
and artifact contents where relevant. Keep tests portable across Linux, macOS
and Windows. Replaying a request can mutate state; use disposable targets.

Read existing tests to understand their assertions before declaring a guarantee
covered. Add integration coverage when correctness spans package boundaries.

## Verification

Run relevant tests during development; before completion run where supported:

```bash
go fmt ./...
go mod tidy
go test ./...
go test -race ./...
go vet ./...
go build ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
git diff --check
```

Review formatting and module diffs; do not introduce unrelated upgrades. CI uses
the Go version in go.mod, all three desktop OS runners, and Linux race/vulnerability
checks. For release changes, use the packaging and verification commands in
[installation/release notes](../docs/installation.md). Never execute incompatible
architecture binaries without an appropriate runner/emulator.

Documentation-only edits, build metadata and exploratory investigations may not
have meaningful unit tests. Use concrete alternatives: validate examples, inspect
links, build/install with the declared toolchain, extract archives and smoke-test
executables. Record commands, outcomes and environmental blockers accurately.
