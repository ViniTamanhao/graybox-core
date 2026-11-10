# Graybox

Graybox records real API behavior so you can reproduce it and detect behavioral
regressions after changing your application.

A local CLI records HTTP traffic into portable `.graybox` files. No account,
daemon, or cloud service required.

```text
record → inspect → change → diff → verify
```

## Try it: an authenticated order API

The [example API](examples/server) returns a $50 order with a $5 discount.
It requires an `Authorization` header and uses only Go's standard library.
Run these commands from the repository root with Go and curl installed.

Build Graybox, then start the API in terminal 1:

```bash
go build -o graybox ./cmd/graybox
go run ./examples/server
```

In terminal 2, start the recorder (use a new output filename if it already exists):

```bash
./graybox record --target http://127.0.0.1:8080 --output order.graybox
```

In terminal 3, send a real authenticated request through Graybox's proxy:

```bash
export API_AUTH='Bearer demo-token'
curl --fail-with-body http://127.0.0.1:9000/orders/42 -H "Authorization: $API_AUTH"
```

The response has `"total_cents":4500`. Stop the recorder with Ctrl+C in terminal 2,
then inspect the recording in terminal 3:

```bash
./graybox ls order.graybox
./graybox show order.graybox 1
```

The stored `Authorization` value is `<REDACTED>`; the successful response remains
available for comparison.

Stop the API with Ctrl+C in terminal 1, then start the intentional regression:

```bash
go run ./examples/server --regressed
```

In terminal 3, compare the changed API with the recording:

```bash
./graybox diff order.graybox --secret-header Authorization=API_AUTH
```

Graybox reads the runtime credential from `API_AUTH` and reports:

```text
Diffing 1 exchanges against http://127.0.0.1:8080
Ignoring: response.headers.date, response.headers.content-length

1 GET /orders/42
  changed
  response.body#/total_cents
    4500 -> 5000

0 equivalent
1 changed
0 failed
```

The exit code is `1` because behavior changed. Restart
the API without `--regressed`, and run the same diff command: it reports `1 equivalent` and exits `0`.
The recording stays unchanged. See the [demo instructions](examples/server/README.md)
for the full walkthrough and authenticated replay.

## Installation

The demo above uses a source checkout. To install the released v0.3.0 CLI
with Go 1.26.10 or newer:

```bash
go install github.com/ViniTamanhao/graybox-core/cmd/graybox@v0.3.0
graybox version
```

Prebuilt binaries do not require Go. See [installation](docs/installation.md)
for downloads and checksum verification.

To run the demo, clone the repository and use the local build shown above:

```bash
git clone https://github.com/ViniTamanhao/graybox-core.git
cd graybox-core
```

Prebuilt binaries are available from [GitHub Releases](https://github.com/ViniTamanhao/graybox-core/releases).
See [installation](docs/installation.md) for pinned installs, PATH setup, checksum
verification and platform-specific extraction. Checkout builds report `dev`;
module installs and release binaries report their version.

## Core commands

| Command | Purpose |
| --- | --- |
| `graybox record` | Proxy and record HTTP traffic |
| `graybox ls` | List recorded exchanges |
| `graybox show` | Inspect one exchange |
| `graybox replay` | Send recorded requests again |
| `graybox diff` | Replay and compare responses with the recording |

Run `graybox help <command>` for options.

## Use with Codex

v0.3.0 includes a portable Codex verification skill alongside the CLI.
Install the CLI and copy the matching [v0.3.0 skill package](docs/agent-skill.md)
into your application's `.agents/skills/graybox`. Codex can discover the skill
for relevant API changes, or you can invoke `$graybox` explicitly. Installation
does not guarantee automatic activation. The skill supplements tests with historical HTTP
evidence; the bounded experiments did not establish improved final correctness.

## OAuth bodies and project configuration

Optional `graybox.yaml` rules extend default header redaction to custom headers,
JSON fields, forms, and query parameters. For example:

```yaml
redact:
  json: [/client_secret, /refresh_token]
replay:
  json:
    /client_secret: "${CLIENT_SECRET}"
    /refresh_token: "${REFRESH_TOKEN}"
```

Record with that file in your current directory, then export `CLIENT_SECRET` and
`REFRESH_TOKEN`. Both `graybox replay session.graybox` and
`graybox diff session.graybox` reconstruct outgoing requests without changing the
sanitized recording. `--config FILE` selects another file; `--secret-header`
overrides configured headers. Missing body/query fields are skipped, so one
configuration works for mixed OAuth and ordinary API sessions. Only applicable
field rules need credentials. JSON paths use the existing JSON Pointer convention.
New recordings retain non-sensitive redaction field lists so diff can apply the
capture and current policies consistently without modifying the recording.
See [Configuration](docs/configuration.md) for complete OAuth, form, query, and
header examples, validation, precedence, and supported formats.

## Security

Graybox redacts `Authorization`, `Proxy-Authorization`, `Cookie`, and `Set-Cookie`
headers during capture. Custom rules supplement these defaults. Unconfigured
fields, unsupported bodies, and URL paths may still contain secrets;
review recordings before sharing them. Keep recordings and reports private and
outside Git. Replay and diff send real requests and can repeat writes: use an
authorized disposable environment with real integrations disabled.
Use `--secret-header HEADER=ENV_VAR` to supply credentials at runtime; those values
are never written back into the recording. Read [Security](docs/SECURITY.md).

## Documentation

- [Installation and release checks](docs/installation.md)
- [Configuration](docs/configuration.md): redaction and runtime credentials
- [Behavioral diffing](docs/diffing.md): comparisons, ignores, exit codes, JSON output
- [Codex verification skill](docs/agent-skill.md): installation, safe use and evaluation evidence
- [Agent CLI failure-handling evaluation](docs/agent-cli-reliability.md)
- [End-to-end agent development evaluation](docs/agent-end-to-end-evaluation.md)
- [Agent verification contract](docs/agent-verification-contract.md) and [workflow](docs/agent-verification-workflow.md)
- [Recording format](docs/recording-format.md)
- [Architecture](docs/architecture.md)
- [Contributing](docs/CONTRIBUTING.md)

MIT licensed. See [LICENSE](LICENSE).
