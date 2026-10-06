# Authenticated order demo

One endpoint, no dependencies beyond Go's standard library:
`GET /orders/42` requires `Authorization: Bearer demo-token` and returns a $50
order with a $5 discount. The token and order are fixed demo data. Nothing is
written by the API, so replay is safe to repeat.

The exercise captures the successful response, introduces a missing-discount
bug, and uses Graybox to find the exact changed JSON field.

Run all commands from the repository root. You need Go 1.26.6 or newer, curl,
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

In [main.go](main.go), replace this line:

```go
total := subtotal - discount
```

with:

```go
total := subtotal
```

This simulates a pricing change accidentally dropping the discount from the total.
Stop the API with Ctrl+C in terminal 1, then restart:

```bash
go run ./examples/server
```

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

Restore `total := subtotal - discount`, stop and restart the API in terminal 1,
then rerun the diff command in terminal 3. It reports `1 equivalent`, `0 changed`,
`0 failed` and exits `0`.

To replay the authenticated request without comparing it:

```bash
./graybox replay order.graybox --secret-header Authorization=API_AUTH
```

Stop the API with Ctrl+C when finished. See [behavioral diffing](../../docs/diffing.md)
and [security](../../docs/SECURITY.md) for details.
