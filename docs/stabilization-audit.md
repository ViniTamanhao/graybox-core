# Phase 1 stabilization audit

Audit started 2026-11-06. Initial checkout: `2240f1b`; fetched main and branch
base: `fdfbf3f`. The intervening changes only update Go to 1.26.10 and YAML to
3.0.5. No existing root agent instructions were present.

## Baseline evidence

Read README, all existing docs, workflows, example, implementation and focused
tests across capture, CLI, configuration, replay, sanitization, diff and storage.
The core workflow already exists; this pass extends it rather than replacing it.

On the original checkout, Go 1.27.3 linux/amd64 passed `go test ./...`,
`go test -race ./...`, `go vet ./...`, `go build ./...`, and
`go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...` (no vulnerabilities).
`go fmt ./...` and `go mod tidy` produced no changes. Initial sandbox attempts
failed on read-only Git metadata and Go cache; authorized execution resolved
these. SSH fetch stalled; HTTPS fetched main successfully.

GitHub evidence inspected directly:

- [main CI](https://github.com/ViniTamanhao/graybox-core/actions/runs/41512893846)
  completed successfully at `fdfbf3f`.
- Latest published release is [v0.2.3](https://github.com/ViniTamanhao/graybox-core/releases/tag/v0.2.3),
  published 2026-10-11, with six OS/architecture archives and SHA256SUMS.
- [v0.2.3 release workflow](https://github.com/ViniTamanhao/graybox-core/actions/runs/38155370235)
  completed successfully. This is older than main's Go security update; this PR
  does not publish a replacement release.

## Confirmed inconsistencies and coverage gaps

- README and example require Go 1.26.6, but fetched `go.mod` requires 1.26.10.
- Demo requires editing source to introduce and revert a regression.
- Release workflow builds six targets but executes only Linux AMD64 before
  packaging; no extraction/checksum/help smoke test gates publication.
- Existing OAuth integration tests check JSON/form/query/header redaction,
  runtime injection, output scrubbing and byte-identical recording preservation.
  They do not exercise the whole baseline/change/restore cycle through `record`.
- Existing tests cover malformed config/pointers, missing/empty credentials with
  zero requests, legacy policy inference, transport/partial body failures,
  truncation, binary/malformed JSON, missing IDs and schema validation. Additional
  explicit HTTP error-status, cancellation and repeated side-effect coverage is
  useful; HTTP application errors must not become transport failures.

## Implementation and verification

Updated as each stage is completed below. No schema or exit-code change is planned.

## Deferred scope

Stateful correlation, token refresh, resource cleanup, automatic rollback and
side-effect prevention remain outside Phase 1. Sequential replay is not a
transaction or a sandbox. Unconfigured secrets and arbitrary server encodings
are not covered by runtime-value scrubbing. No claim of universal secret safety
or production readiness follows from passing tests.

### HTTP final-status correction

`TestProxyEarlyHintsPreservesFinalResponse` failed before the fix: a 103 followed
by 201 reached the client as 200 and was stored as 103. The capture writer now
forwards informational responses without committing its final status. The test
checks client and SQLite status, full body and absence of interim-only headers;
`go test ./internal/capture -count=1` passes after the fix.

### Authenticated workflow and failure coverage

Added a complete CLI record/shutdown/inspect/replay/diff/change/restore test using
an authenticated loopback server. It independently queries SQLite columns for
request identity, status, body and mandatory header markers; searches raw file
bytes for synthetic canaries; checks structured semantic values and exit codes;
and compares artifact bytes after repeated human and JSON invocations.
Existing OAuth tests cover custom headers, nested/array JSON, forms, queries,
response redaction and percent-encoded runtime values; existing hardening tests
cover JSON escapes, preflight, old policy-less recordings and output sanitization.
Added explicit 401/403/500 completed-comparison tests, empty/invalid recording
and missing-ID checks, cancellation/deadline/refused-connection checks and a POST
repetition test demonstrating real side effects. No universal secret-detection
claim is made. The example now accepts `--regressed` and requires authentication
in both modes; returning to baseline requires only restarting without the flag.

### Redacted request markers

A focused test confirmed that both commands previously transmitted exact
`<REDACTED>` JSON/form/query values without replacements and could report false
workflow equivalence. This contradicted the documented reconstruction rule.
Replay now checks the outgoing supported representation after replacement and
refuses remaining marker fields before HTTP execution. Legacy recordings are
covered without requiring metadata. Existing OAuth replacement tests still pass.
This is a narrow safety correction: literal marker fields now fail too; CLI flags,
schema and numerical exit-code meanings remain unchanged. Unsupported formats
are not an arbitrary-secret detection surface.

### Release and installation evidence

Extracted the existing build loop into `scripts/package-release.sh` (same six
targets, filenames and linker version interface). Python standard-library ZIP
creation removes the need for an extra Windows ZIP utility; there are no new Go
runtime dependencies. The verifier checks the exact archive/checksum manifest,
root executable and mode, then runs only the native binary's startup/help and
text/JSON version commands. CI runs native packaging on each OS; publication
waits for checks of the same archives on Linux, macOS and Windows.

All six local targets cross-built and passed archive/checksum verification;
Linux AMD64 executable checks passed. Deliberately tampered archives and extra
distribution files were rejected. An earlier all-target invocation was interrupted
by editing its script while it ran; the clean final-script rerun passed.
Downloaded all six published v0.2.3 archives and SHA256SUMS independently: all
checksums/layouts passed, and the Linux AMD64 executable reported v0.2.3 with
working help/JSON version. No foreign executable was run locally. Pinned
`go install .../cmd/graybox@v0.2.3` into a temporary GOBIN succeeded and reported
`graybox v0.2.3`. Exact-minimum `GOTOOLCHAIN=go1.26.10 go test ./...` passed.
