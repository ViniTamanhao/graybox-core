# Security

## Recordings are sensitive

Graybox observes application traffic. A `.graybox` recording may contain authentication tokens, cookies, passwords, personal data, API keys, private identifiers, internal URLs, proprietary payloads, and other secrets.

Graybox automatically replaces values in these headers before writing them:

- `Authorization`
- `Proxy-Authorization`
- `Cookie`
- `Set-Cookie`

Header matching is case-insensitive and the stored marker is `<REDACTED>`. Reconstructed requests omit redacted headers rather than sending the marker.

This policy is not exhaustive. In particular, Graybox does not inspect or redact request/response bodies, URL paths or queries, other potentially sensitive headers, or recorded proxy-error text. Treat every raw `.graybox` file as potentially sensitive. Review a recording before sharing it, use restrictive filesystem permissions, and avoid committing recordings to source control.

Body capture is bounded (10 MiB per request or response by default), but that bound is a resource limit, not a privacy control. Each body stores its captured byte prefix plus `observed_size`, `captured_size`, `truncated`, and `complete`; secrets can occur within the retained prefix. An interrupted response is recorded when possible with `complete = false` and a non-empty `proxy_error`. A Graybox-generated transport 502 also has a non-empty `proxy_error`, while an upstream 502 does not.

Graybox is local-first and never uploads recordings automatically.

The recorder binds to `127.0.0.1:9000` by default. A wildcard address such as `:9000` or `0.0.0.0:9000` can expose the proxy to the local network; use one only when that exposure is intended and protected by appropriate host controls.

## Replay and diff safety

Both `graybox replay` and `graybox diff` can send recorded payloads to a server. Graybox automatically reuses a saved target only for `localhost`, `*.localhost` names, and loopback IP addresses. A saved remote target requires an explicit `--target` or `--unsafe-original-target`. Review the destination and recording before allowing either command to contact it.

Redirects are reported without being followed. Truncated or incomplete recorded request bodies are refused rather than sent partially. Redacted credentials are omitted rather than transmitted, and recorded hop-by-hop headers are not replayed.

For V1 diffing, detailed live responses pass through the same response-header sanitizer before their headers enter comparison results. This prevents automatically redacted live response headers such as `Set-Cookie` from being exposed by diff output. It does not make diff output safe to share: secrets can still appear in response bodies, URLs, application-specific headers, other non-redacted values, human diff output, and JSON diff output. Treat all diff output as potentially sensitive.

## Reporting a vulnerability

Please report suspected vulnerabilities privately through GitHub's security-advisory feature for this repository. Include the affected version, impact, and a minimal reproduction when possible. Do not include real credentials or private production recordings.
