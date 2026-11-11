# README demonstration provenance

`order-diff.svg` is an original, hand-composed SVG containing an excerpt of actual
Graybox human output. It is a presentation of a real comparison, not a terminal
screenshot. No Bend media, branding or wording was copied. No generated bitmap,
recording, credential file or binary is included.

Verified November 11, 2026 on Linux AMD64, Go 1.27.3, against source based on
`f4412b330bc65a79798229a110257833002f13f4`. Only the public synthetic
`Bearer demo-token` credential and disposable local processes were used.

The [walkthrough](../../examples/server/README.md) supplies these commands:

```bash
go build -o graybox ./cmd/graybox
go run ./examples/server
./graybox record --target http://127.0.0.1:8080 --output order.graybox
export API_AUTH='Bearer demo-token'
curl --fail-with-body http://127.0.0.1:9000/orders/42 -H "Authorization: $API_AUTH"
./graybox ls order.graybox
./graybox show order.graybox 1
go run ./examples/server --regressed
./graybox diff order.graybox --secret-header Authorization=API_AUTH
```

These are separate-terminal steps, not one script: stop the recorder before
inspection, and stop the API before each restart. The verification harness built
the same API into a temporary executable for precise process ownership; the
example's `--regressed` flag changes behavior without changing source. A later
run without that flag restored equivalence. The exact diff command, default ports
and output were executed; authenticated replay and JSON comparison also passed.
Stored Authorization was redacted and the recording bytes stayed unchanged.
All owned services were stopped. Temporary recordings/binaries stayed outside Git.

The SVG omits the target/ignore preamble and preserves the exchange and summary
lines verbatim. Color emphasizes the observed difference without implying that
all changes are bugs. A fixed opaque background works in both GitHub themes;
README alt text and adjacent prose give a textual equivalent at narrow widths.
