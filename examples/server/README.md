# Authenticated order demo

One endpoint, no dependencies beyond Go's standard library:
`GET /orders/42` requires `Authorization: Bearer demo-token` and returns a $50
order with a $5 discount. The token and order are fixed demo data. Nothing is
written by the API, so replay is safe to repeat.

The exercise captures the successful response, introduces a missing-discount
bug, and uses Graybox to find the exact changed JSON field.

Run all commands from the repository root. You need Go 1.26.10 or newer, curl,
and three terminals. Ports 8080 (API) and 9000 (recorder) must be free.

## 1. Start the API and recorder

Terminal 1:

```bash
go build -o graybox ./cmd/graybox
go run ./examples/server
```

Terminal 2:

```bash
./graybox record --target http://127.0.0.1:8080 --output order.graybox
```

Wait for both processes to print their startup messages. Graybox refuses to
replace an existing recording; use a new filename if repeating the capture.

## 2. Record and inspect

Terminal 3:

```bash
export API_AUTH='Bearer demo-token'
curl --fail-with-body http://127.0.0.1:9000/orders/42 -H "Authorization: $API_AUTH"
```

Expected response:

```json
{"id":42,"currency":"USD","subtotal_cents":5000,"discount_cents":500,"total_cents":4500}
```

Stop the recorder with Ctrl+C in terminal 2. In terminal 3:

```bash
./graybox ls order.graybox
./graybox show order.graybox 1
```

Look for `200 OK`, `Authorization: <REDACTED>`, and `"total_cents": 4500`.
The upstream API received the real credential; Graybox redacted it before storage.

## 3. Change and restart the API

Stop the API with Ctrl+C in terminal 1, then restart with the explicit demo flag:

```bash
go run ./examples/server --regressed
```

This intentionally omits the discount from the total without editing source.
The endpoint and authentication stay the same.

## 4. Diff with a runtime credential

In terminal 3 (where `API_AUTH` is still exported):

```bash
./graybox diff order.graybox --secret-header Authorization=API_AUTH
```

The changed field is:

```text
1 GET /orders/42
  changed
  response.body#/total_cents
    4500 -> 5000
```

The summary is `0 equivalent`, `1 changed`, `0 failed`; exit code `1` means a
behavioral difference. Both responses are successful HTTP responses, but the
new total is wrong. Graybox reuses the recorded loopback target and supplies
the credential only in the outgoing request. The recording stays redacted.

Without `--secret-header`, the stored credential is omitted and the API returns
`401`; that would compare authentication failure rather than the pricing change.

## 5. Fix and verify

Stop the API and restart with `go run ./examples/server` (no `--regressed`) in terminal 1,
then rerun the diff command in terminal 3. It reports `1 equivalent`, `0 changed`,
`0 failed` and exits `0`.

To replay the authenticated request without comparing it:

```bash
./graybox replay order.graybox --secret-header Authorization=API_AUTH
```

Stop the API with Ctrl+C when finished. See [behavioral diffing](../../docs/diffing.md)
and [security](../../docs/SECURITY.md) for details.


## Repeat or automate

After capture, reuse the same immutable recording for comparisons. To capture
again, stop the old recorder and choose a fresh `--output` filename. Stop the
previous API before switching modes so ports are released. To use another API
port, set `GRAYBOX_EXAMPLE_LISTEN=127.0.0.1:8081` and pass the matching recorder
`--target`. These shell examples use Bash; on PowerShell set credentials with
`$env:API_AUTH = 'Bearer demo-token'` and run `.\graybox.exe`.

The automated local workflow runs without fixed ports or external credentials:

```bash
go test ./internal/cli -run TestAuthenticatedLifecycle -count=1
go test ./examples/server -count=1
```

It verifies capture, stored redaction, authenticated replay, a changed JSON total,
restored equivalence, JSON output, exit codes and unchanged recording bytes. The
example handler tests independently verify baseline/regressed prices and auth.
