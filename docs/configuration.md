# Configurable redaction and runtime credentials

Graybox v0.2.2 optionally reads `graybox.yaml` from the current working directory
for `record`, `replay`, and `diff`. It does not search parent directories. Select
another file with `--config FILE` on any of those commands; an explicit file must
exist and replaces the discovered file entirely. No configuration is required.
`ls`, `show`, and `version` do not load configuration or credentials.

## OAuth-style JSON requests

Create `graybox.yaml` before recording:

```yaml
redact:
  headers:
    - X-Custom-Secret
  json:
    - /client_secret
    - /refresh_token
    - /credentials/api_key
    - /items/0/token
  query:
    - access_token
  form:
    - client_secret
    - refresh_token

replay:
  headers:
    Authorization: "Bearer ${API_TOKEN}"
  json:
    /client_secret: "${CLIENT_SECRET}"
    /refresh_token: "${REFRESH_TOKEN}"
  query:
    access_token: "${ACCESS_TOKEN}"
```

Record against your local API:

```bash
graybox record --target http://localhost:8080 --output session.graybox
```

For an OAuth-style JSON request body, configured values are stored as:

```json
{
  "client_id": "my-app",
  "client_secret": "<REDACTED>",
  "refresh_token": "<REDACTED>"
}
```

The original request and response still pass through the proxy unchanged. Stop
the recorder, then export current credentials and run either command:

```bash
export API_TOKEN='current-api-token'
export CLIENT_SECRET='current-client-secret'
export REFRESH_TOKEN='current-refresh-token'
export ACCESS_TOKEN='current-query-token'

graybox replay session.graybox
graybox diff session.graybox
```

The query replacement in this example requires `access_token` in every selected
request. Remove that mapping if your requests authenticate only through the
header and body. Only configure the replacements needed by your recording.

## Fields and formats

| Location | Selection | Behavior |
| --- | --- | --- |
| Headers | Case-insensitive HTTP header name | Every repeated value is redacted; replay sets a single runtime value |
| JSON | Non-root RFC 6901 JSON Pointer | Selects exactly one existing object field or array element |
| Query | Exact, case-sensitive decoded parameter name | Every occurrence of the selected parameter is redacted or replaced |
| Form | Exact, case-sensitive decoded field name | Every occurrence of the selected field is redacted or replaced |

JSON uses the same pointer syntax as diff's `response.body#/pointer` ignores,
without the `response.body#` prefix. `/credentials/api_key` selects a nested
field, `/items/0/token` selects the first array element's field, and `/items/1`
selects an entire array element. Within tokens, escape `~` as `~0` and `/` as
`~1`; `/a~1b/~0key` selects the key `~key` inside `a/b`. `/` selects an empty
object key. Dotted paths, JSONPath, wildcards, root replacement, negative or
leading-zero array indices, and array append syntax are unsupported. Overlapping
JSON replacement paths are rejected.

JSON redaction/replacement applies to UTF-8 `application/json` and `+json` media
types. Forms use `application/x-www-form-urlencoded`, including on responses.
Media type parameters such as `charset=utf-8` are accepted. JSON values selected
for redaction become the string `<REDACTED>`, even when originally numbers,
objects, or arrays. Runtime JSON replacements are **always strings**. Unrelated
scalar types, large numbers, arrays, and object structure are preserved; JSON
formatting and key order may change. A JSON-looking environment value is still
a string, with no parsing or type inference.

Forms and queries are decoded and encoded with Go's standard URL encoding;
spaces, `+`, `&`, `/`, and repeated values are handled correctly. Configuring query
rules or runtime query replacement normalizes query encoding/order. With no
query rules or replacements, the original raw query is retained.

## Form authentication

For a form-encoded token request, replace the JSON replay section with:

```yaml
replay:
  form:
    client_secret: "${CLIENT_SECRET}"
    refresh_token: "${REFRESH_TOKEN}"
```

Keep the matching `redact.form` rules. Both JSON and form replay sections can be
configured together; the request's `Content-Type` selects the applicable body
section. Configured body replacements require a matching supported media type.
Every configured field in that section must exist in each selected request.
Use `--id` or a separate explicit configuration when a recording contains
requests with different authentication field sets.

## Resolution, precedence, and failures

Replay values require exactly one `${ENV_VAR}` reference, optionally surrounded
by literal text such as `Bearer `. Environment names use letters, digits, and
underscores and cannot start with a digit. Resolved values stay in execution
memory; the parsed configuration retains only the original references.
`record` validates configuration but does not resolve replay credentials.

`--secret-header HEADER=ENV_VAR` remains supported and overrides a configured
replay header case-insensitively. The flag reads the complete header value, so
its environment variable must include `Bearer ` when needed. An overridden
configuration reference is not resolved. Duplicate CLI mappings are rejected.
CLI target, selection, and ignore flags keep their existing behavior.

Unknown keys, duplicate keys, non-string field names/values, invalid pointers,
malformed YAML, aliases, extra YAML documents, unsupported replay headers, and
invalid environment references fail with usage exit code `4`. Missing or empty
required environment variables and invalid header values fail before any HTTP
request is sent. Configuration diagnostics omit file contents and resolved
values. A missing request field or invalid request body fails that exchange
before sending it, with existing replay/diff failure exit codes. Later exchanges
are still processed.

Redaction and replacement are independent: configured redaction need not have a
replacement, and a replacement can override an unredacted field. Header
replacements may add a header; JSON, form, and query replacements require an
existing target. The outgoing body length is recalculated. Recording files are
opened read-only and never modified by replay or diff.

## Security and comparison limits

The mandatory `Authorization`, `Proxy-Authorization`, `Cookie`, and `Set-Cookie`
redaction rules always apply to request and response headers. Custom rules add
protection. JSON and form rules apply to both request and response bodies; query
rules apply to the recorded request URL. Missing redaction fields are ignored.
Live diff responses use the configured header/body rules before comparison;
keep the capture configuration available for consistent comparisons. Ignore
rules and comparison semantics are unchanged.

When body rules are configured, malformed, non-UTF-8, compressed, unsupported,
truncated, or incomplete nonempty bodies that cannot safely be sanitized are
withheld as `<REDACTED>` and marked incomplete. Replay refuses withheld requests;
diff cannot establish equivalence for withheld responses unless `response.body`
is ignored. Supported formats with no matching body rules remain uninspected.
Malformed queries with query rules withhold the entire query. With configured
redaction, capture transport/stream/persistence diagnostics use generic messages
rather than potentially sensitive upstream error text.

Schema 1 remains unchanged. For transformed bodies, sizes describe the sanitized
representation: `captured_size` is its byte length and `observed_size` adds any
uncaptured original bytes. These sizes are not the original wire length. The
original truncation flag is retained. See [Recording format](recording-format.md).

Injected runtime values are scrubbed from human/JSON output and errors, including
JSON escaping and URL/form encoding. This is value scrubbing, not detection of
arbitrary secrets or arbitrary server transformations of credentials. Unsupported
formats, URL paths, unconfigured fields, and unrelated server secrets may still
contain sensitive data. Recordings and diff output are **not universally safe to
share**. Review them before sharing. There is no `.env` loading, token refresh,
response-to-request token extraction, vault, or OAuth orchestration.
