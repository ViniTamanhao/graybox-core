# Graybox

A local-first HTTP flight recorder for reproducing and comparing API behavior.

Graybox records real HTTP traffic into portable `.graybox` files. You can inspect what happened, replay the original requests, then compare the current behavior after making a change.

No account, daemon, or cloud service required.

```text
record → inspect → fix → diff → verify
```

## Quick start

Run the included example API:

```bash
go run ./examples/server
```

Start Graybox in another terminal:

```bash
graybox record \
  --target http://localhost:8080 \
  --output bug.graybox
```

Send traffic through the proxy:

```bash
curl http://127.0.0.1:9000/hello

curl -X POST http://127.0.0.1:9000/echo \
  -H 'Content-Type: application/json' \
  -d '{"message":"hello"}'
```

Stop the recorder with Ctrl+C.

Inspect what happened:

```bash
graybox ls bug.graybox
graybox show bug.graybox 1
```

After changing the application, compare its current behavior with the recording:

```bash
graybox diff bug.graybox
```

Example:

```text
Diffing 2 exchanges against http://localhost:8080
Ignoring: response.headers.date, response.headers.content-length

1 GET /hello
  equivalent

2 POST /echo
  changed
  response.body#/message
    "hello" -> "hello world"

1 equivalent
1 changed
0 failed
```

## Install

Graybox requires Go 1.26 or newer when building from source.

```bash
go install github.com/ViniTamanhao/graybox-core/cmd/graybox@latest
```

Or build the repository directly:

```bash
git clone https://github.com/ViniTamanhao/graybox-core.git
cd graybox-core

go build -o graybox ./cmd/graybox
```

Prebuilt binaries for Linux, macOS, and Windows are available from GitHub Releases.

## Commands

| Command           | Purpose                                                          |
| ----------------- | ---------------------------------------------------------------- |
| `graybox record`  | Record HTTP traffic through a local reverse proxy                |
| `graybox ls`      | List recorded exchanges                                          |
| `graybox show`    | Inspect one recorded exchange                                    |
| `graybox replay`  | Replay recorded requests                                         |
| `graybox diff`    | Replay requests and compare current responses with the recording |
| `graybox version` | Print the Graybox version                                        |

Most commands also support `--json` for scripts and tooling.

## Behavioral diffing

`graybox diff` compares response status, headers, and bodies.

Complete JSON responses are compared semantically, so formatting, object-key order, and equivalent numbers such as `1` and `1.0` do not create false differences.

Non-JSON bodies are compared byte-for-byte.

Volatile values can be ignored explicitly:

```bash
graybox diff bug.graybox \
  --ignore 'response.body#/metadata/request_id'
```

`Date` and `Content-Length` response headers are ignored by default.

A result can be:

* `equivalent` — comparison completed and no differences were found
* `changed` — comparison completed and behavior changed
* `failed` — the comparison could not be completed

See [Behavioral diffing](docs/diffing.md) for the full comparison model, ignore syntax, exit codes, and JSON format.

## Recordings

A `.graybox` file is an ordinary SQLite database.

Graybox stores the effective HTTP request, observed response, headers, timing, bounded body captures, and metadata needed for replay.

Body capture is bounded to 10 MiB per request or response by default. Traffic continues streaming after the capture limit; the recording keeps the retained prefix together with size, truncation, and completion metadata.

Recording schema 1 was introduced in `v0.1.0` and remains the format used by `v0.2.0`.

See [Recording format](docs/recording-format.md).

## Replay safety

Replay and diff can send recorded requests to a server.

When `--target` is omitted, Graybox automatically reuses the recorded target only for localhost and loopback addresses. Remote recorded targets require an explicit `--target` or `--unsafe-original-target`.

Redirects are not followed, redacted credentials are not sent, and truncated or incomplete request bodies are refused.

## Security

Recordings may contain sensitive application data.

Graybox automatically redacts values from:

* `Authorization`
* `Proxy-Authorization`
* `Cookie`
* `Set-Cookie`

This is deliberately limited. Bodies, URLs, and other application-specific values may still contain secrets.

Read [Security](docs/SECURITY.md) before sharing recordings or diff output.

## Documentation

* [Behavioral diffing](docs/diffing.md)
* [Architecture](docs/architecture.md)
* [Recording format](docs/recording-format.md)
* [Security](docs/SECURITY.md)
* [Contributing](docs/CONTRIBUTING.md)

## Scope

Graybox is focused on local HTTP debugging and behavioral verification.

It is not an APM, packet analyzer, service mesh, transparent proxy, or hosted API client. Features such as gRPC-specific decoding, mocking, regression suites, CI integrations, and agent integrations may be added as the project develops.

## Contributing

Focused issues and pull requests are welcome. See [Contributing](docs/CONTRIBUTING.md).

## License

MIT. See [LICENSE](LICENSE).
