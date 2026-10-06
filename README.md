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

Introduce a bug in [examples/server/main.go](examples/server/main.go): change
`total := subtotal - discount` to `total := subtotal`. Stop the API with Ctrl+C
in terminal 1, then restart it with `go run ./examples/server`.

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

The exit code is `1` because behavior changed. Restore the subtraction, restart
the API, and run the same diff command: it reports `1 equivalent` and exits `0`.
The recording stays unchanged. See the [demo instructions](examples/server/README.md)
for the full walkthrough and authenticated replay.

## Installation

Building from source requires Go 1.26.6 or newer:

```bash
go install github.com/ViniTamanhao/graybox-core/cmd/graybox@latest
```

To run the demo, clone the repository and use the local build shown above:

```bash
git clone https://github.com/ViniTamanhao/graybox-core.git
cd graybox-core
```

Prebuilt binaries are available from [GitHub Releases](https://github.com/ViniTamanhao/graybox-core/releases).

## Core commands

| Command | Purpose |
| --- | --- |
| `graybox record` | Proxy and record HTTP traffic |
| `graybox ls` | List recorded exchanges |
| `graybox show` | Inspect one exchange |
| `graybox replay` | Send recorded requests again |
| `graybox diff` | Replay and compare responses with the recording |

Run `graybox help <command>` for options.

## Security

Graybox redacts `Authorization`, `Proxy-Authorization`, `Cookie`, and `Set-Cookie`
headers during capture. Bodies, URLs, and other headers may still contain secrets;
review recordings before sharing them. Replay and diff send real requests.
Use `--secret-header HEADER=ENV_VAR` to supply credentials at runtime; those values
are never written back into the recording. Read [Security](docs/SECURITY.md).

## Documentation

- [Behavioral diffing](docs/diffing.md): comparisons, ignores, exit codes, JSON output
- [Recording format](docs/recording-format.md)
- [Architecture](docs/architecture.md)
- [Contributing](docs/CONTRIBUTING.md)

MIT licensed. See [LICENSE](LICENSE).
