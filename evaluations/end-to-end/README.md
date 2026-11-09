# Disposable commerce development evaluation

Read [protocol.md](protocol.md) before running trials. This fixture is evaluation
code, not a production example or a new Graybox engine. Requires Go matching the
root go.mod, Python 3.9+, Git, and Linux bubblewrap for isolated live trials.
Live inference additionally requires an authenticated Codex CLI. No external
commerce services, notifications, payments or real customer data are used.

The Go fixture uses the repository's existing modernc SQLite driver. Products
1/2/3 cost 1999/5/10000 cents. Orders 40/41/42 belong to Alice/Bob/Alice and are
pending/pending/shipped. Creation starts at ID 43. SAVE10 rounds half up; quantities
are 1–100. JSON rejects unknown fields and trailing values. Runtime API_AUTH and
OTHER_AUTH identify the two synthetic owners; do not print their values.

GET /products/{id}, POST /discounts/apply, POST /orders, GET /orders/{id}, and
POST /orders/{id}/cancel use authenticated JSON. Quote POSTs are pure; creation
and cancellation mutate SQLite. No retry, rollback across HTTP calls, dynamic-ID
substitution, idempotency protocol or external integrations exist.

Build locally with `go build -o /tmp/commerce ./evaluations/end-to-end/app`.
With synthetic runtime headers provisioned in API_AUTH and OTHER_AUTH, run
`/tmp/commerce --db /private/path/commerce.sqlite --reset` while stopped, then
`/tmp/commerce --db /private/path/commerce.sqlite`. Startup prints its OS-selected
loopback URL. Stop only the process you launched and observe termination.
Reset drops this fixture's tables, so never point it at an unrelated database.

The runner prepares fresh private directories, captures reference traffic before
seeding defects, preserves provenance/digests and independently grades HTTP.
Never commit databases, recordings, binaries, raw reports or conversations.
Acceptance code remains evaluator-side, absent from trial mounts and Git history.
See the runner's help and evaluation report for executed commands and limitations.
