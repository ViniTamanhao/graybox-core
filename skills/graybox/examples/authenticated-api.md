# Authenticated order exercise

This manual exercise uses Graybox's public synthetic
[order example](https://github.com/ViniTamanhao/graybox-core/tree/main/examples/server),
not a production API. The skill itself needs no source checkout; this optional
exercise does. From an inspected Graybox source checkout, build the CLI and API
into a private temporary directory, and use three terminals. Go and curl are
required; ports 8080 and 9000 must be free. No real external integrations exist
in this example. The deliberate `--regressed` mode is a test, not a discovered bug.

Terminal 1:

```bash
umask 077
demo_dir="$(mktemp -d /tmp/graybox-demo.XXXXXXXXXX)" || exit 1
go build -o "$demo_dir/graybox" ./cmd/graybox
go build -o "$demo_dir/api" ./examples/server
printf 'Use this demo_dir in other terminals: %s\n' "$demo_dir"
"$demo_dir/api"
```

Terminal 2: set `demo_dir` to the printed path. Start capture:

```bash
"$demo_dir/graybox" record --target http://127.0.0.1:8080 \
  --output "$demo_dir/order.graybox"
```

After startup, terminal 3 (same `demo_dir`):

```bash
export API_AUTH='Bearer demo-token' # Public synthetic demo value only.
curl --fail-with-body http://127.0.0.1:9000/orders/42 -H "Authorization: $API_AUTH"
```

Expected `total_cents` is 4500. Stop only the recorder with Ctrl+C in terminal 2
and check its final exit is 0. In terminal 3 inspect the baseline:

```bash
"$demo_dir/graybox" ls "$demo_dir/order.graybox" --json
"$demo_dir/graybox" show "$demo_dir/order.graybox" 1 --json
```

Confirm response 200, complete/nontruncated bodies, redacted Authorization, and
reference total 4500. Preserve the file and record its SHA-256 with your platform's
digest tool. Set this shell function in terminal 3 to retain all three channels:

```bash
compare_order() {
  if "$demo_dir/graybox" diff "$demo_dir/order.graybox" --id 1 \
    --target http://127.0.0.1:8080 --secret-header Authorization=API_AUTH --json \
    > "$demo_dir/$1.json" 2> "$demo_dir/$1.stderr"; then
    result=0
  else
    result=$?
  fi
  printf 'Graybox exit: %s\n' "$result"
  cat "$demo_dir/$1.json"
}
compare_order equivalent
```

Expect exit 0, total 1/equivalent 1. Stop API with Ctrl+C in terminal 1 and run:

```bash
"$demo_dir/api" --regressed
```

Terminal 3: `compare_order intentional-regression`. Expect exit 1, changed 1,
`location.component` = `response.body`, `location.path` = `/total_cents`,
`before.value` = 4500 and `after.value` = 5000. Both HTTP responses are 200.

Stop API, restart `"$demo_dir/api"` without the flag, then run
`compare_order restored`. Expect exit 0, equivalent 1. Confirm baseline digest
unchanged. Report only this selected sanitized response under default ignores.
Stop API and remove the private demo directory when finished reviewing evidence.
No real credential belongs in a transcript or report. A deterministic version of
this lifecycle is included in the repository's evaluation harness; neither this
walkthrough nor that script proves autonomous agent behavior.
