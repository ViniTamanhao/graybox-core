# Security

## Recordings are sensitive

Graybox observes application traffic. A `.graybox` recording may contain authentication tokens, cookies, passwords, personal data, API keys, private identifiers, internal URLs, proprietary payloads, and other secrets.

During capture, Graybox automatically replaces values in these headers before writing them:

- `Authorization`
- `Proxy-Authorization`
- `Cookie`
- `Set-Cookie`

Header matching is case-insensitive and the stored marker is `<REDACTED>`. Reconstructed requests omit redacted headers rather than sending the marker.

This policy is not exhaustive. Optional [configuration](configuration.md) adds custom header, JSON Pointer, query, and form redaction. Default protections cannot be disabled. URL paths, unconfigured fields, and supported formats without matching rules remain uninspected. Unsupported or malformed bodies with configured body protection are withheld and marked incomplete; without body rules, their captured bytes remain unchanged. Configured capture uses generic failure diagnostics to avoid retaining potentially sensitive upstream error text. Treat every raw `.graybox` file as potentially sensitive. Review a recording before sharing it, use restrictive filesystem permissions, and avoid committing recordings to source control.

Body capture is bounded (10 MiB per request or response by default), but that bound is a resource limit, not a privacy control. Each body stores its captured bytes (sanitized or withheld when configured) plus `observed_size`, `captured_size`, `truncated`, and `complete`; secrets can occur within the retained prefix. An interrupted response is recorded when possible with `complete = false` and a non-empty `proxy_error`. A Graybox-generated transport 502 also has a non-empty `proxy_error`, while an upstream 502 does not.

Graybox is local-first and never uploads recordings automatically.

The recorder binds to `127.0.0.1:9000` by default. A wildcard address such as `:9000` or `0.0.0.0:9000` can expose the proxy to the local network; use one only when that exposure is intended and protected by appropriate host controls.

## Replay and diff safety

Both `graybox replay` and `graybox diff` can send recorded payloads to a server. Graybox automatically reuses a saved target only for `localhost`, `*.localhost` names, and loopback IP addresses. A saved remote target requires an explicit `--target` or `--unsafe-original-target`. Review the destination and recording before allowing either command to contact it.

Redirects are reported without being followed. Truncated or incomplete recorded request bodies are refused rather than sent partially. Redacted recorded headers are omitted rather than restored automatically; body/query markers require applicable runtime replacements, and recorded hop-by-hop headers are not replayed. Current credentials require runtime configuration or explicit header mappings as described below.

Diff sanitizes both the in-memory baseline and live response with the same effective policy. New captures retain optional `redaction_policy` metadata containing only field lists; diff combines it with current configuration without allowing stored protection to be disabled. Policy metadata contains no credentials, runtime mappings, or environment references. Legacy recordings without metadata infer exact sensitive locations from baseline markers; absent fields cannot reveal their old rules, so supply the capture configuration for those recordings. This prevents automatically redacted live response headers such as `Set-Cookie` from being exposed by diff output. It does not make diff output safe to share: secrets can still appear in response bodies, URLs, application-specific headers, other non-redacted values, human diff output, and JSON diff output. Treat all diff output as potentially sensitive.

## Runtime credentials

Recordings remain historical artifacts with standard recorded credentials kept redacted. Runtime credentials are execution configuration, supplied explicitly to `replay` or `diff` through optional `graybox.yaml` replay mappings or repeatable `--secret-header HEADER=ENV_VAR` flags:

```bash
export API_AUTH='Bearer abc123'
export API_KEY='secret-api-key'

graybox diff bug.graybox \
  --secret-header Authorization=API_AUTH \
  --secret-header X-API-Key=API_KEY
```

The same flags work with `graybox replay`. Graybox reads values from the named environment variables; it does not guess variable names or restore recorded credentials. These values override the corresponding recorded headers only in outgoing replay/diff HTTP requests. They are never written back into the `.graybox` recording.

Runtime secret values supplied through configuration or `--secret-header` are scrubbed from Graybox-generated human output, JSON output, and errors, including known JSON/JSON Pointer escapes, HTTP header-name normalization, and upper/lowercase URL/form percent encodings. Application values are scrubbed before display truncation. Matching numeric/boolean application values become marker strings; Graybox's schema fields retain their types. Configured protection also withholds potentially sensitive transport diagnostics. This guarantee applies specifically to those runtime values, not to general secret detection. Unrelated secrets already present in response bodies, URLs, application-specific headers, recorded data, or other values can still appear in output. Review output before sharing it.

Capture-time redaction and runtime secret injection are separate concerns. Supplying `--secret-header X-API-Key=API_KEY` does not redact an `X-API-Key` in the original captured request or retroactively sanitize the recording. Configure `redact.headers` to protect additional headers during capture. The mandatory automatic capture-time header list is `Authorization`, `Proxy-Authorization`, `Cookie`, and `Set-Cookie`.

Body/query rules are applied only to existing fields in safely parsed selected requests. A preflight resolves credentials for those applicable rules; unused field mappings do not demand environment variables. Header mappings keep their global behavior. Empty or unrelated supported bodies remain unchanged. Duplicate-key JSON, malformed data, compressed bodies, and unsupported nonempty bodies fail when applicability cannot safely be established; fields hidden beneath a redacted ancestor and withheld queries are also refused. Failed transformations never fall back to persisting or sending raw sensitive data. Capture-time failures with body rules withhold the body and mark it incomplete.

Configuration errors fail before any HTTP request is sent and use Graybox's usage exit code `4`. These include malformed mappings, missing or empty environment variables, duplicate mappings for the same header (case-insensitively), invalid HTTP header names, invalid environment variable names, invalid header values, and unsupported transport or special headers.

Transport and special headers cannot be supplied through `--secret-header`:

- `Connection`
- `Content-Length`
- `Host`
- `Keep-Alive`
- `Proxy-Authenticate`
- `Proxy-Authorization`
- `Proxy-Connection`
- `TE`
- `Trailer`
- `Transfer-Encoding`
- `Upgrade`

`Authorization` is explicitly allowed, as are normal request headers such as `X-API-Key`.

Configuration can restore JSON body, form, and query credentials using `${ENV_VAR}` references. JSON values are strings; forms and queries are URL encoded. A CLI header mapping takes precedence over a configured header. No resolved value is written into the parsed configuration or recording. See [Configuration](configuration.md) for format limits, withheld-body behavior, and validation.

## Reporting a vulnerability

Please report suspected vulnerabilities privately through GitHub's security-advisory feature for this repository. Include the affected version, impact, and a minimal reproduction when possible. Do not include real credentials or private production recordings.
